package main

import (
	"os"
	"encoding/json"
	"fmt"
	"errors"
	"crypto/subtle"
)

type Config struct {
	AllowedConnections   map[string]string `json:"allowed_connections"` // team name -> token
	SpectatorToken       string            `json:"spectator_token"`
	MasterSpectatorToken string            `json:"master_spectator_token"`
}

type authTable struct {
	playerByToken  map[string]string // token -> team name
	spectatorToken string
	masterToken    string
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cfg, nil
}

func newAuthTable(cfg *Config) (*authTable, error) {
	if cfg.SpectatorToken == "" || cfg.MasterSpectatorToken == "" {
		return nil, errors.New("spectator and master tokens must be set")
	}
	if cfg.SpectatorToken == cfg.MasterSpectatorToken {
		return nil, errors.New("spectator and master tokens must differ")
	}

	a := &authTable{
		playerByToken:  make(map[string]string, len(cfg.AllowedConnections)),
		spectatorToken: cfg.SpectatorToken,
		masterToken:    cfg.MasterSpectatorToken,
	}
	for team, tok := range cfg.AllowedConnections {
		switch {
			case tok == "":
				return nil, fmt.Errorf("team %q has an empty token", team)
			case tok == a.spectatorToken || tok == a.masterToken:
				return nil, fmt.Errorf("team %q reuses a spectator/master token", team)
		}
		if other, dup := a.playerByToken[tok]; dup {
			return nil, fmt.Errorf("teams %q and %q share a token", team, other)
		}
		a.playerByToken[tok] = team
	}
	return a, nil
}

func (a *authTable) getRole(token string) (role Role, ok bool) {
	if subtle.ConstantTimeCompare([]byte(token), []byte(a.masterToken)) == 1 {
		return RoleMasterSpectator, true
	}
	if token == a.spectatorToken {
		return RoleSpectator, true
	}
	if _, ok := a.playerByToken[token]; ok {
		return RolePlayer, true
	}
	return 0, false
}
