package configs

import (
	"fmt"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

var cfgPtr atomic.Pointer[Config]
var cfgWriteMu sync.Mutex

type UpdateRejectedError struct {
	Err error
}

func (e *UpdateRejectedError) Error() string {
	return e.Err.Error()
}

func (e *UpdateRejectedError) Unwrap() error {
	return e.Err
}

type PersistError struct {
	Err error
}

func (e *PersistError) Error() string {
	return e.Err.Error()
}

func (e *PersistError) Unwrap() error {
	return e.Err
}

// GetCfg 返回当前配置（只读视角）；更新配置请使用 UpdateCfg/UpdateCfgAndSaveToDB。
func GetCfg() *Config {
	return cfgPtr.Load()
}

func MustGetCfg() *Config {
	cfg := GetCfg()
	if cfg == nil {
		panic("config is not initialized")
	}
	return cfg
}

func StoreCfg(cfg *Config) {
	cfgPtr.Store(cfg)
}

func UpdateCfg(update func(cfg *Config) error) error {
	cfgWriteMu.Lock()
	defer cfgWriteMu.Unlock()

	old := cfgPtr.Load()
	if old == nil {
		return fmt.Errorf("config is not initialized")
	}

	next, err := cloneConfig(old)
	if err != nil {
		return err
	}
	if err := update(next); err != nil {
		return &UpdateRejectedError{Err: err}
	}

	cfgPtr.Store(next)
	return nil
}

func UpdateCfgAndSaveToDB(dbi ConfigDBInterface, update func(cfg *Config) error) error {
	cfgWriteMu.Lock()
	defer cfgWriteMu.Unlock()

	old := cfgPtr.Load()
	if old == nil {
		return fmt.Errorf("config is not initialized")
	}

	next, err := cloneConfig(old)
	if err != nil {
		return err
	}
	if err := update(next); err != nil {
		return &UpdateRejectedError{Err: err}
	}

	if err := next.SaveToDB(dbi); err != nil {
		return &PersistError{Err: err}
	}

	cfgPtr.Store(next)
	return nil
}

func cloneConfig(cfg *Config) (*Config, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var cloned Config
	if err := yaml.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}
