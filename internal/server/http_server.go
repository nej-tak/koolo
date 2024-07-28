package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hectorgimenez/d2go/pkg/data"
	"github.com/hectorgimenez/d2go/pkg/data/area"
	"github.com/hectorgimenez/d2go/pkg/data/difficulty"
	"github.com/hectorgimenez/d2go/pkg/data/stat"
	koolo "github.com/hectorgimenez/koolo/internal"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/helper"
)

type HttpServer struct {
	logger    *slog.Logger
	server    *http.Server
	manager   *koolo.SupervisorManager
	templates *template.Template
	wsServer  *WebSocketServer
}

var (
	//go:embed all:assets
	assetsFS embed.FS
	//go:embed all:templates
	templatesFS embed.FS

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type WebSocketServer struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

func NewWebSocketServer() *WebSocketServer {
	return &WebSocketServer{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (s *WebSocketServer) Run() {
	for {
		select {
		case client := <-s.register:
			s.clients[client] = true
		case client := <-s.unregister:
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				close(client.send)
			}
		case message := <-s.broadcast:
			for client := range s.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(s.clients, client)
				}
			}
		}
	}
}

func (s *WebSocketServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade connection to WebSocket", "error", err)
		return
	}

	client := &Client{conn: conn, send: make(chan []byte, 256)}
	s.register <- client

	go s.writePump(client)
	go s.readPump(client)
}

func (s *WebSocketServer) writePump(client *Client) {
	defer func() {
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			if !ok {
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := client.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

func (s *WebSocketServer) readPump(client *Client) {
	defer func() {
		s.unregister <- client
		client.conn.Close()
	}()

	for {
		_, _, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("WebSocket read error", "error", err)
			}
			break
		}
	}
}

func (s *HttpServer) BroadcastStatus() {
	for {
		data := s.getStatusData()
		jsonData, err := json.Marshal(data)
		if err != nil {
			slog.Error("Failed to marshal status data", "error", err)
			continue
		}

		s.wsServer.broadcast <- jsonData
		time.Sleep(1 * time.Second)
	}
}

func New(logger *slog.Logger, manager *koolo.SupervisorManager) (*HttpServer, error) {
	var templates *template.Template
	helperFuncs := template.FuncMap{
		"isInSlice": func(slice []stat.Resist, value string) bool {
			return slices.Contains(slice, stat.Resist(value))
		},
		"isTZSelected": func(slice []area.ID, value int) bool {
			return slices.Contains(slice, area.ID(value))
		},
		"executeTemplateByName": func(name string, data interface{}) template.HTML {
			tmpl := templates.Lookup(name)
			var buf bytes.Buffer
			if tmpl == nil {
				return "This run is not configurable."
			}

			tmpl.Execute(&buf, data)
			return template.HTML(buf.String())
		},
		"qualityClass": qualityClass,
		"statIDToText": statIDToText,
		"contains":     containss,
	}
	templates, err := template.New("").Funcs(helperFuncs).ParseFS(templatesFS, "templates/*.gohtml")
	if err != nil {
		return nil, err
	}

	return &HttpServer{
		logger:    logger,
		manager:   manager,
		templates: templates,
	}, nil
}

func qualityClass(quality string) string {
	switch quality {
	case "LowQuality":
		return "low-quality"
	case "Normal":
		return "normal-quality"
	case "Superior":
		return "superior-quality"
	case "Magic":
		return "magic-quality"
	case "Set":
		return "set-quality"
	case "Rare":
		return "rare-quality"
	case "Unique":
		return "unique-quality"
	default:
		return "unknown-quality"
	}
}

func statIDToText(id stat.ID) string {
	return stat.StringStats[id]
}

func containss(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func getAvailableTZs() map[int]string {
	availableTZs := make(map[int]string)
	for _, tz := range area.Areas {
		if tz.CanBeTerrorized() {
			availableTZs[int(tz.ID)] = tz.Name
		}
	}
	return availableTZs
}

func (s *HttpServer) initialData(w http.ResponseWriter, r *http.Request) {
	if config.Koolo.Overseer.Enabled {
		enableCors(&w)
	}
	data := s.getStatusData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *HttpServer) getStatusData() IndexData {
	status := make(map[string]koolo.Stats)
	drops := make(map[string]int)

	for _, supervisorName := range s.manager.AvailableSupervisors() {
		status[supervisorName] = s.manager.Status(supervisorName)
		if s.manager.GetSupervisorStats(supervisorName).Drops != nil {
			drops[supervisorName] = len(s.manager.GetSupervisorStats(supervisorName).Drops)
		} else {
			drops[supervisorName] = 0
		}
	}

	return IndexData{
		Version:   config.Version,
		Status:    status,
		DropCount: drops,
	}
}

func (s *HttpServer) Listen(port int) error {
	s.wsServer = NewWebSocketServer()
	go s.wsServer.Run()
	go s.BroadcastStatus()

	http.HandleFunc("/", s.getRoot)
	http.HandleFunc("/config", s.config)
	http.HandleFunc("/supervisorSettings", s.characterSettings)
	http.HandleFunc("/start", s.startSupervisor)
	http.HandleFunc("/stop", s.stopSupervisor)
	http.HandleFunc("/stop-all", s.stopAllSupervisors)
	http.HandleFunc("/togglePause", s.togglePause)
	http.HandleFunc("/debug", s.debugHandler)
	http.HandleFunc("/debug-data", s.debugData)
	http.HandleFunc("/drops", s.drops)
	http.HandleFunc("/ws", s.wsServer.HandleWebSocket) // Web socket
	http.HandleFunc("/initial-data", s.initialData)    // Web socket data

	if config.Koolo.Overseer.Enabled {
		ServeOverseerAPI(s)
	}

	assets, _ := fs.Sub(assetsFS, "assets")
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))

	s.server = &http.Server{
		Addr: fmt.Sprintf(":%d", port),
	}

	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func (s *HttpServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

func (s *HttpServer) getRoot(w http.ResponseWriter, r *http.Request) {
	if !helper.HasAdminPermission() {
		s.templates.ExecuteTemplate(w, "templates/admin_required.gohtml", nil)
		return
	}

	if config.Koolo.FirstRun {
		http.Redirect(w, r, "/config", http.StatusSeeOther)
		return
	}

	s.index(w)
}

func (s *HttpServer) debugData(w http.ResponseWriter, r *http.Request) {
	characterName := r.URL.Query().Get("characterName")
	if characterName == "" {
		http.Error(w, "Character name is required", http.StatusBadRequest)
		return
	}
	gameData := s.manager.GetData(characterName)
	jsonData, err := json.Marshal(gameData)
	if err != nil {
		http.Error(w, "Failed to serialize game data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func (s *HttpServer) debugHandler(w http.ResponseWriter, r *http.Request) {
	s.templates.ExecuteTemplate(w, "debug.gohtml", nil)
}

func (s *HttpServer) startSupervisor(w http.ResponseWriter, r *http.Request) {
	supervisorList := s.manager.AvailableSupervisors()
	Supervisor := r.URL.Query().Get("characterName")

	// Get the current auth method for the supervisor we wanna start
	supCfg, currFound := config.Characters[Supervisor]
	if !currFound {
		// There's no config for the current supervisor. THIS SHOULDN'T HAPPEN
		return
	}

	// Prevent launching of other clients while there's a client with TokenAuth still starting
	for _, sup := range supervisorList {

		// If the current don't check against the one we're trying to launch
		if sup == Supervisor {
			continue
		}

		if s.manager.GetSupervisorStats(sup).SupervisorStatus == koolo.Starting {

			// Prevent launching if we're using token auth & another client is starting (no matter what auth method)
			if supCfg.AuthMethod == "TokenAuth" {
				return
			}

			// Prevent launching if another client that is using token auth is starting
			sCfg, found := config.Characters[sup]
			if found {
				if sCfg.AuthMethod == "TokenAuth" {
					return
				}
			}
		}
	}

	s.manager.Start(Supervisor)
	s.initialData(w, r)
}

func (s *HttpServer) stopAllSupervisors(w http.ResponseWriter, r *http.Request) {
	s.manager.StopAllByName()
	s.initialData(w, r)
}

func (s *HttpServer) stopSupervisor(w http.ResponseWriter, r *http.Request) {
	s.manager.Stop(r.URL.Query().Get("characterName"))
	s.initialData(w, r)
}

func (s *HttpServer) togglePause(w http.ResponseWriter, r *http.Request) {
	s.manager.TogglePause(r.URL.Query().Get("characterName"))
	s.initialData(w, r)
}

func (s *HttpServer) index(w http.ResponseWriter) {
	status := make(map[string]koolo.Stats)
	drops := make(map[string]int)

	for _, supervisorName := range s.manager.AvailableSupervisors() {
		status[supervisorName] = koolo.Stats{
			SupervisorStatus: koolo.NotStarted,
		}

		status[supervisorName] = s.manager.Status(supervisorName)

		if s.manager.GetSupervisorStats(supervisorName).Drops != nil {
			drops[supervisorName] = len(s.manager.GetSupervisorStats(supervisorName).Drops)
		} else {
			drops[supervisorName] = 0
		}

	}

	s.templates.ExecuteTemplate(w, "index.gohtml", IndexData{
		Version:   config.Version,
		Status:    status,
		DropCount: drops,
	})
}

func (s *HttpServer) drops(w http.ResponseWriter, r *http.Request) {
	sup := r.URL.Query().Get("supervisor")
	cfg, found := config.Characters[sup]
	if !found {
		http.Error(w, "Can't fetch drop data because the configuration "+sup+" wasn't found", http.StatusNotFound)
		return
	}

	var Drops []data.Drop

	if s.manager.GetSupervisorStats(sup).Drops == nil {
		Drops = make([]data.Drop, 0)
	} else {
		Drops = s.manager.GetSupervisorStats(sup).Drops
	}

	s.templates.ExecuteTemplate(w, "drops.gohtml", DropData{
		NumberOfDrops: len(Drops),
		Character:     cfg.CharacterName,
		Drops:         Drops,
	})
}

func (s *HttpServer) config(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			s.templates.ExecuteTemplate(w, "config.gohtml", ConfigData{KooloCfg: config.Koolo, ErrorMessage: "Error parsing form"})
			return
		}

		newConfig := *config.Koolo
		newConfig.FirstRun = false // Disable the welcome assistant
		newConfig.D2RPath = r.Form.Get("d2rpath")
		newConfig.D2LoDPath = r.Form.Get("d2lodpath")
		newConfig.UseCustomSettings = r.Form.Get("use_custom_settings") == "true"
		newConfig.GameWindowArrangement = r.Form.Get("game_window_arrangement") == "true"
		// Debug
		newConfig.Debug.Log = r.Form.Get("debug_log") == "true"
		newConfig.Debug.Screenshots = r.Form.Get("debug_screenshots") == "true"
		// Discord
		newConfig.Discord.Enabled = r.Form.Get("discord_enabled") == "true"
		newConfig.Discord.EnableGameCreatedMessages = r.Form.Has("enable_game_created_messages")
		newConfig.Discord.EnableNewRunMessages = r.Form.Has("enable_new_run_messages")
		newConfig.Discord.EnableRunFinishMessages = r.Form.Has("enable_run_finish_messages")
		newConfig.Discord.Token = r.Form.Get("discord_token")
		newConfig.Discord.ChannelID = r.Form.Get("discord_channel_id")
		// Overseer
		newConfig.Overseer.Enabled = r.Form.Get("overseer_enabled") == "true"
		newConfig.Overseer.AppURL = r.Form.Get("overseer_app_url")
		// Telegram
		newConfig.Telegram.Enabled = r.Form.Get("telegram_enabled") == "true"
		newConfig.Telegram.Token = r.Form.Get("telegram_token")
		telegramChatId, err := strconv.ParseInt(r.Form.Get("telegram_chat_id"), 10, 64)
		if err != nil {
			s.templates.ExecuteTemplate(w, "config.gohtml", ConfigData{KooloCfg: &newConfig, ErrorMessage: "Invalid Telegram Chat ID"})
			return
		}
		newConfig.Telegram.ChatID = telegramChatId

		err = config.ValidateAndSaveConfig(newConfig)
		if err != nil {
			s.templates.ExecuteTemplate(w, "config.gohtml", ConfigData{KooloCfg: &newConfig, ErrorMessage: err.Error()})
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.templates.ExecuteTemplate(w, "config.gohtml", ConfigData{KooloCfg: config.Koolo, ErrorMessage: ""})
}

func (s *HttpServer) characterSettings(w http.ResponseWriter, r *http.Request) {
	var err error
	if r.Method == http.MethodPost {
		err = r.ParseForm()
		if err != nil {
			s.templates.ExecuteTemplate(w, "character_settings.gohtml", CharacterSettings{
				ErrorMessage: err.Error(),
			})

			return
		}

		supervisorName := r.Form.Get("name")
		cfg, found := config.Characters[supervisorName]
		if !found {
			err = config.CreateFromTemplate(supervisorName)
			if err != nil {
				s.templates.ExecuteTemplate(w, "character_settings.gohtml", CharacterSettings{
					ErrorMessage: err.Error(),
					Supervisor:   supervisorName,
				})

				return
			}
			cfg = config.Characters["template"]
		}

		cfg.MaxGameLength, _ = strconv.Atoi(r.Form.Get("maxGameLength"))
		cfg.CharacterName = r.Form.Get("characterName")
		cfg.CommandLineArgs = r.Form.Get("commandLineArgs")
		cfg.KillD2OnStop = r.Form.Has("kill_d2_process")
		cfg.ClassicMode = r.Form.Has("classic_mode")
		cfg.CloseMiniPanel = r.Form.Has("close_mini_panel")

		// Bnet config
		cfg.Username = r.Form.Get("username")
		cfg.Password = r.Form.Get("password")
		cfg.Realm = r.Form.Get("realm")
		cfg.AuthMethod = r.Form.Get("authmethod")
		cfg.AuthToken = r.Form.Get("AuthToken")

		// Health config
		cfg.Health.HealingPotionAt, _ = strconv.Atoi(r.Form.Get("healingPotionAt"))
		cfg.Health.ManaPotionAt, _ = strconv.Atoi(r.Form.Get("manaPotionAt"))
		cfg.Health.RejuvPotionAtLife, _ = strconv.Atoi(r.Form.Get("rejuvPotionAtLife"))
		cfg.Health.RejuvPotionAtMana, _ = strconv.Atoi(r.Form.Get("rejuvPotionAtMana"))
		cfg.Health.ChickenAt, _ = strconv.Atoi(r.Form.Get("chickenAt"))
		cfg.Character.UseMerc = r.Form.Has("useMerc")
		cfg.Health.MercHealingPotionAt, _ = strconv.Atoi(r.Form.Get("mercHealingPotionAt"))
		cfg.Health.MercRejuvPotionAt, _ = strconv.Atoi(r.Form.Get("mercRejuvPotionAt"))
		cfg.Health.MercChickenAt, _ = strconv.Atoi(r.Form.Get("mercChickenAt"))
		// Character
		cfg.Character.Class = r.Form.Get("characterClass")
		cfg.Character.StashToShared = r.Form.Has("characterStashToShared")
		cfg.Character.UseTeleport = r.Form.Has("characterUseTeleport")

		for y, row := range cfg.Inventory.InventoryLock {
			for x := range row {
				if r.Form.Has(fmt.Sprintf("inventoryLock[%d][%d]", y, x)) {
					cfg.Inventory.InventoryLock[y][x] = 0
				} else {
					cfg.Inventory.InventoryLock[y][x] = 1
				}
			}
		}

		for x, value := range r.Form["inventoryBeltColumns[]"] {
			cfg.Inventory.BeltColumns[x] = value
		}

		// Game
		cfg.Game.MinGoldPickupThreshold, _ = strconv.Atoi(r.Form.Get("gameMinGoldPickupThreshold"))
		cfg.Game.Difficulty = difficulty.Difficulty(r.Form.Get("gameDifficulty"))
		cfg.Game.RandomizeRuns = r.Form.Has("gameRandomizeRuns")

		// Runs specific config

		enabledRuns := make([]config.Run, 0)
		// we don't like errors, so we ignore them
		json.Unmarshal([]byte(r.FormValue("gameRuns")), &enabledRuns)
		cfg.Game.Runs = enabledRuns

		cfg.Game.Cows.OpenChests = r.Form.Has("gameCowsOpenChests")

		cfg.Game.Pit.MoveThroughBlackMarsh = r.Form.Has("gamePitMoveThroughBlackMarsh")
		cfg.Game.Pit.OpenChests = r.Form.Has("gamePitOpenChests")
		cfg.Game.Pit.FocusOnElitePacks = r.Form.Has("gamePitFocusOnElitePacks")

		cfg.Game.Andariel.ClearRoom = r.Form.Has("gameAndarielClearRoom")

		cfg.Game.Pindleskin.SkipOnImmunities = []stat.Resist{}
		for _, i := range r.Form["gamePindleskinSkipOnImmunities[]"] {
			cfg.Game.Pindleskin.SkipOnImmunities = append(cfg.Game.Pindleskin.SkipOnImmunities, stat.Resist(i))
		}

		cfg.Game.StonyTomb.OpenChests = r.Form.Has("gameStonytombOpenChests")
		cfg.Game.StonyTomb.FocusOnElitePacks = r.Form.Has("gameStonytombFocusOnElitePacks")
		cfg.Game.AncientTunnels.OpenChests = r.Form.Has("gameAncientTunnelsOpenChests")
		cfg.Game.AncientTunnels.FocusOnElitePacks = r.Form.Has("gameAncientTunnelsFocusOnElitePacks")
		cfg.Game.Mephisto.KillCouncilMembers = r.Form.Has("gameMephistoKillCouncilMembers")
		cfg.Game.Mephisto.OpenChests = r.Form.Has("gameMephistoOpenChests")
		cfg.Game.Tristram.ClearPortal = r.Form.Has("gameTristramClearPortal")
		cfg.Game.Tristram.FocusOnElitePacks = r.Form.Has("gameTristramFocusOnElitePacks")
		cfg.Game.Nihlathak.ClearArea = r.Form.Has("gameNihlathakClearArea")

		cfg.Game.Baal.KillBaal = r.Form.Has("gameBaalKillBaal")
		cfg.Game.Baal.DollQuit = r.Form.Has("gameBaalDollQuit")
		cfg.Game.Baal.SoulQuit = r.Form.Has("gameBaalSoulQuit")
		cfg.Game.Baal.ClearFloors = r.Form.Has("gameBaalClearFloors")
		cfg.Game.Baal.OnlyElites = r.Form.Has("gameBaalOnlyElites")

		cfg.Game.Eldritch.KillShenk = r.Form.Has("gameEldritchKillShenk")
		cfg.Game.Diablo.ClearArea = r.Form.Has("gameDiabloClearArea")
		cfg.Game.Diablo.OnlyElites = r.Form.Has("gameDiabloOnlyElites")
		cfg.Game.Diablo.KillDiablo = r.Form.Has("gameDiabloKillDiablo")
		cfg.Game.Leveling.EnsurePointsAllocation = r.Form.Has("gameLevelingEnsurePointsAllocation")
		cfg.Game.Leveling.EnsureKeyBinding = r.Form.Has("gameLevelingEnsureKeyBinding")

		// Quests options for Act 1
		cfg.Game.Quests.ClearDen = r.Form.Has("gameQuestsClearDen")
		cfg.Game.Quests.RescueCain = r.Form.Has("gameQuestsRescueCain")
		cfg.Game.Quests.RetrieveHammer = r.Form.Has("gameQuestsRetrieveHammer")
		// Quests options for Act 2
		cfg.Game.Quests.KillRadament = r.Form.Has("gameQuestsKillRadament")
		cfg.Game.Quests.GetCube = r.Form.Has("gameQuestsGetCube")
		// Quests options for Act 3
		cfg.Game.Quests.RetrieveBook = r.Form.Has("gameQuestsRetrieveBook")
		// Quests options for Act 4
		cfg.Game.Quests.KillIzual = r.Form.Has("gameQuestsKillIzual")
		// Quests options for Act 5
		cfg.Game.Quests.KillShenk = r.Form.Has("gameQuestsKillShenk")
		cfg.Game.Quests.RescueAnya = r.Form.Has("gameQuestsRescueAnya")
		cfg.Game.Quests.KillAncients = r.Form.Has("gameQuestsKillAncients")

		cfg.Game.TerrorZone.FocusOnElitePacks = r.Form.Has("gameTerrorZoneFocusOnElitePacks")
		cfg.Game.TerrorZone.SkipOtherRuns = r.Form.Has("gameTerrorZoneSkipOtherRuns")

		cfg.Game.TerrorZone.SkipOnImmunities = []stat.Resist{}
		for _, i := range r.Form["gameTerrorZoneSkipOnImmunities[]"] {
			cfg.Game.TerrorZone.SkipOnImmunities = append(cfg.Game.TerrorZone.SkipOnImmunities, stat.Resist(i))
		}

		tzAreas := make([]area.ID, 0)
		for _, a := range r.Form["gameTerrorZoneAreas[]"] {
			ID, _ := strconv.Atoi(a)
			tzAreas = append(tzAreas, area.ID(ID))
		}
		cfg.Game.TerrorZone.Areas = tzAreas

		// Gambling
		cfg.Gambling.Enabled = r.Form.Has("gamblingEnabled")

		// Cube Recipes
		cfg.CubeRecipes.Enabled = r.Form.Has("enableCubeRecipes")
		enabledRecipes := r.Form["enabledRecipes"]
		cfg.CubeRecipes.EnabledRecipes = enabledRecipes
		// Companion

		// Companion config
		cfg.Companion.Enabled = r.Form.Has("companionEnabled")
		cfg.Companion.Leader = r.Form.Has("companionLeader")
		cfg.Companion.Attack = r.Form.Has("companionAttack")
		cfg.Companion.FollowLeader = r.Form.Has("companionFollowLeader")
		cfg.Companion.LeaderName = r.Form.Get("companionLeaderName")
		cfg.Companion.GameNameTemplate = r.Form.Get("companionGameNameTemplate")
		cfg.Companion.GamePassword = r.Form.Get("companionGamePassword")

		// Back to town config
		cfg.BackToTown.NoHpPotions = r.Form.Has("noHpPotions")
		cfg.BackToTown.NoMpPotions = r.Form.Has("noMpPotions")
		cfg.BackToTown.MercDied = r.Form.Has("mercDied")
		cfg.BackToTown.EquipmentBroken = r.Form.Has("equipmentBroken")

		config.SaveSupervisorConfig(supervisorName, cfg)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	supervisor := r.URL.Query().Get("supervisor")
	cfg := config.Characters["template"]
	if supervisor != "" {
		cfg = config.Characters[supervisor]
	}

	enabledRuns := make([]string, 0)
	// Let's iterate cfg.Game.Runs to preserve current order
	for _, run := range cfg.Game.Runs {
		enabledRuns = append(enabledRuns, string(run))
	}
	disabledRuns := make([]string, 0)
	for run := range config.AvailableRuns {
		if !slices.Contains(cfg.Game.Runs, run) {
			disabledRuns = append(disabledRuns, string(run))
		}
	}

	availableTZs := getAvailableTZs()

	s.templates.ExecuteTemplate(w, "character_settings.gohtml", CharacterSettings{
		Supervisor:   supervisor,
		Config:       cfg,
		EnabledRuns:  enabledRuns,
		DisabledRuns: disabledRuns,
		AvailableTZs: availableTZs,
		RecipeList:   config.AvailableRecipes,
	})
}
