package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type ServiceState struct {
	Hash       string    `json:"hash"`
	BuildHash  string    `json:"buildHash,omitempty"`
	DeployedAt time.Time `json:"deployedAt"`
	ImageTag   string    `json:"imageTag,omitempty"`
}

type State struct {
	Services map[string]ServiceState `json:"services"`
}

func New() *State {
	return &State{
		Services: make(map[string]ServiceState),
	}
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	if s.Services == nil {
		s.Services = make(map[string]ServiceState)
	}
	return &s, nil
}

func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}

func (s *State) HasChanged(name, hash string) bool {
	svc, ok := s.Services[name]
	if !ok {
		return true
	}
	return svc.Hash != hash
}

func (s *State) Update(name string, ss ServiceState) {
	s.Services[name] = ss
}

func (s *State) Get(name string) (ServiceState, bool) {
	ss, ok := s.Services[name]
	return ss, ok
}
