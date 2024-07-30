package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hectorgimenez/koolo/internal/config"
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

func getSanitizedConfigs() map[string]*config.CharacterCfg {
	dst := make(map[string]*config.CharacterCfg)
	for key, value := range config.Characters {
		copy := *value
		copy.Username, copy.Password, copy.AuthToken = "", "", ""
		dst[key] = &copy
	}
	return dst
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", config.Koolo.Overseer.AppURL)
}

func (s *HttpServer) GetKooloConfig(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	if err := config.Load(); err != nil {
		fmt.Printf("Failed to load configurations: %v\n", err)
		return
	}

	data := KooloConfigResponse{
		KooloConfig: config.Koolo,
	}
	w.Header().Set("Content-Type", "application/json")
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

func ServeOverseerAPI(s *HttpServer) {
	http.HandleFunc("/overseer/config/koolo", s.GetKooloConfig)
	http.HandleFunc("/overseer/config/supervisors", s.GetSupervisorConfigs)
	http.HandleFunc("/overseer/available", s.GetAvailableOptions)
}
