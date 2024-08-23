package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/hectorgimenez/d2go/pkg/data"
	koolo "github.com/hectorgimenez/koolo/internal"
	"github.com/hectorgimenez/koolo/internal/config"
	"github.com/hectorgimenez/koolo/internal/event"
)

type KooloConfigResponse struct {
	KooloConfig *config.KooloCfg `json:"koolo"`
}

type SupervisorConfigsResponse struct {
	SupervisorConfigs map[string]*config.CharacterCfg `json:"supervisors"`
}

type AvailableOptions struct {
	AvailableRuns    map[config.Run]interface{} `json:"runs"`
	AvailableRecipes []string                   `json:"recipes"`
	AvailableTZs     map[int]string             `json:"tzs"`
}

type OsGameData struct {
	Data       data.Data `json:"data"`
	Seed       string    `json:"seed"`
	Difficulty string    `json:"difficulty"`
}

func ServeOverseerAPI(s *HttpServer) {
	http.HandleFunc("/overseer/test", s.Test)
	http.HandleFunc("/overseer/config/koolo", s.KooloConfig)
	http.HandleFunc("/overseer/config/supervisors", s.GetSupervisorConfigs)
	http.HandleFunc("/overseer/config/supervisor", s.SupervisorConfig)
	http.HandleFunc("/overseer/game-data", s.initialGameData)
	http.HandleFunc("/overseer/available", s.GetAvailableOptions)
	http.HandleFunc("/overseer/img", s.ShareScreen)
}

func (s *HttpServer) BroadcastGameData() {
	for {
		gd := make(map[string]OsGameData)

		for _, supervisorName := range s.manager.AvailableSupervisors() {
			st := s.manager.Status(supervisorName).SupervisorStatus
			if st == koolo.InGame || st == koolo.Paused {
				data := s.manager.GetData(supervisorName)
				gd[supervisorName] = OsGameData{
					Data:       data.Data,
					Seed:       s.manager.GetMapSeed(supervisorName),
					Difficulty: string(data.CharacterCfg.Game.Difficulty),
				}
			}
		}

		if len(gd) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		jsonData, err := json.Marshal(gd)
		if err != nil {
			slog.Error("Failed to marshal game data", "error", err)
			continue
		}

		s.wsGameData.gdStream <- jsonData
		time.Sleep(2 * time.Second)
	}
}

func (s *HttpServer) initialGameData(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	supervisorName := r.URL.Query().Get("supervisor")
	if supervisorName == "" {
		http.Error(w, "?supervisor= name is required", http.StatusBadRequest)
		return
	}

	data := s.manager.GetData(supervisorName)

	osGameData := OsGameData{
		Data:       data.Data,
		Seed:       s.manager.GetMapSeed(supervisorName),
		Difficulty: string(data.CharacterCfg.Game.Difficulty),
	}

	jsonData, err := json.Marshal(osGameData)
	if err != nil {
		http.Error(w, "Failed to serialize game data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func (s *WebSocketServer) BroadcastToOverseerWs(message []byte) {
	s.overseer <- message
}

func (s *HttpServer) BroadcastToOverseer(message []byte) {
	if s.wsServerOs != nil {
		s.wsServerOs.BroadcastToOverseerWs(message)
	}
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", config.Koolo.Overseer.AppURL)
	(*w).Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func (s *HttpServer) KooloConfig(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		var updates map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&updates)
		if err != nil {
			http.Error(w, `{"error_message": "Error parsing JSON"}`, http.StatusBadRequest)
			return
		}

		newConfig := *config.Koolo
		newConfig.FirstRun = false

		configValue := reflect.ValueOf(&newConfig).Elem()
		if err := updateConfigField(configValue, updates); err != nil {
			http.Error(w, `{"error_message": "`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		err = config.ValidateAndSaveConfig(newConfig)
		if err != nil {
			response := ConfigData{KooloCfg: &newConfig, ErrorMessage: err.Error()}
			json.NewEncoder(w).Encode(response)
			return
		}

		response := ConfigData{KooloCfg: &newConfig, ErrorMessage: ""}
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := config.Load(); err != nil {
		fmt.Printf("Failed to load configurations: %v\n", err)
		return
	}

	data := KooloConfigResponse{
		KooloConfig: config.Koolo,
	}
	json.NewEncoder(w).Encode(data)
}

func (s *HttpServer) GetSupervisorConfigs(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	if err := config.Load(); err != nil {
		fmt.Printf("Failed to load configurations: %v\n", err)
		return
	}

	data := SupervisorConfigsResponse{
		SupervisorConfigs: getSanitizedConfigs(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *HttpServer) Test(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	//overseer.GetInstance().Api.PostDrop(nil, "test", "", "", nil)

	event.Send(event.OnLog(event.Text("testSupervisor", "log"), "msg via event from route", 0))

	s.wsServerOs.GetOverseerChannel() <- []byte("test route hit")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode("ok")
}

func (s *HttpServer) SupervisorConfig(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	if err := config.Load(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to load configurations: %v"}`, err), http.StatusInternalServerError)
		return
	}

	Supervisor := r.URL.Query().Get("supervisor")

	conf, found := config.Characters[Supervisor]
	if !found {
		http.Error(w, `{"error": "Supervisor not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getSanitizedConfig(conf))
}

func (s *HttpServer) GetAvailableOptions(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	data := AvailableOptions{
		AvailableRuns:    config.AvailableRuns,
		AvailableRecipes: config.AvailableRecipes,
		AvailableTZs:     getAvailableTZs(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *HttpServer) ShareScreen(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	supervisor := r.URL.Query().Get("supervisor")
	if supervisor == "" {
		http.Error(w, "?supervisor= name is required", http.StatusBadRequest)
		return
	}

	for _, supervisorName := range s.manager.AvailableSupervisors() {
		if supervisorName == supervisor {
			status := s.manager.Status(supervisorName).SupervisorStatus
			if status == koolo.InGame || status == koolo.Starting || status == koolo.Paused {
				chr := config.Characters[supervisorName].CharacterName

				imgBytes, err := s.captureImageWithRetry(chr, 10, 300*time.Millisecond)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "image/jpeg")
				w.Write(imgBytes)
				return

			} else {
				http.Error(w, "not ready'", http.StatusBadRequest)
				return
			}
		}
	}
}

func (s *HttpServer) captureImageWithRetry(characterName string, maxRetries int, sleepDuration time.Duration) ([]byte, error) {
	var errorsEncountered []error

	for i := 0; i < maxRetries; i++ {
		img, err := s.manager.GetImg(characterName)
		if err != nil {
			errorsEncountered = append(errorsEncountered, fmt.Errorf("attempt %d: %w", i+1, err))
			time.Sleep(sleepDuration)
			continue
		}

		bb, err := imageToJPEGBytes(img, 50)
		if err != nil {
			errorsEncountered = append(errorsEncountered, fmt.Errorf("attempt %d: %w", i+1, err))
			time.Sleep(sleepDuration)
			continue
		}

		if len(bb) >= 9999 { // shouldnt be needed anymore but jic of encoder fkup
			return bb, nil
		}

		errorsEncountered = append(errorsEncountered, fmt.Errorf("attempt %d: image too small", i+1))
		time.Sleep(sleepDuration)
	}

	// If all retries fail, return a concatenated error
	if len(errorsEncountered) > 0 {
		combinedError := "failed to capture image after multiple attempts:\n"
		for _, err := range errorsEncountered {
			combinedError += fmt.Sprintf("%v\n", err)
		}
		return nil, errors.New(combinedError)
	}

	return nil, errors.New("unknown error")
}

func imageToJPEGBytes(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func updateConfigField(configValue reflect.Value, updates map[string]interface{}) error {
	for field, value := range updates {
		fieldValue := configValue.FieldByName(field)
		if !fieldValue.IsValid() {
			continue
		}

		if !fieldValue.CanSet() {
			return errors.New("field cannot be set: " + field)
		}

		if fieldValue.Kind() == reflect.Struct {
			nestedUpdates, ok := value.(map[string]interface{})
			if !ok {
				return errors.New("type mismatch for field: " + field)
			}
			if err := updateConfigField(fieldValue, nestedUpdates); err != nil {
				return err
			}
		} else {
			val := reflect.ValueOf(value)
			if fieldValue.Type() != val.Type() {
				if fieldValue.Kind() == reflect.Int64 && val.Kind() == reflect.Float64 {
					val = reflect.ValueOf(int64(val.Float()))
				} else if fieldValue.Kind() == reflect.Bool && val.Kind() == reflect.Bool {
				} else if fieldValue.Kind() == reflect.String && val.Kind() == reflect.String {
				} else {
					return errors.New("type mismatch for field: " + field)
				}
			}
			fieldValue.Set(val)
		}
	}
	return nil
}

func getSanitizedConfigs() map[string]*config.CharacterCfg {
	dst := make(map[string]*config.CharacterCfg)
	for key, value := range config.Characters {
		sanitized := getSanitizedConfig(value)
		dst[key] = sanitized
	}
	return dst
}

func getSanitizedConfig(conf *config.CharacterCfg) *config.CharacterCfg {
	copy := *conf
	copy.Username = ""
	copy.Password = ""
	copy.AuthToken = ""

	return &copy
}
