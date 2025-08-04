package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type AppConfig struct {
	IgnorePaths      []string `json:"ignore_paths"`
	Base64NamePrefix string   `json:"base_64_name_prefix"`
}

const configPath = "config/config.json"

var (
	cache       AppConfig
	lastModTime time.Time
	mutex       sync.Mutex
)

func GetConfig() AppConfig {
	mutex.Lock()
	defer mutex.Unlock()

	info, err := os.Stat(configPath)

	if err != nil {
		fmt.Printf("Error getting file info from the configuration file in \"%s\". Error: %s\n", configPath, err.Error())
		fmt.Println("Using cached version")
		return cache
	}

	if info.ModTime().Equal(lastModTime) {
		return cache
	}

	data, err := os.ReadFile(configPath)

	if err != nil {
		fmt.Printf("Error reading the configuration file in \"%s\". Error: %s\n", configPath, err.Error())
		fmt.Println("Using cached version")
		return cache
	}

	var newAppConfig AppConfig

	if err := json.Unmarshal(data, &newAppConfig); err != nil {
		fmt.Printf("Invalid configuration in \"%s\": %s \n", configPath, err.Error())
		fmt.Println("Using cached version")
		return cache
	}

	if newAppConfig.Base64NamePrefix == "" {
		fmt.Println("Invalid configuration value for \"base_64_name_prefix\". Expected: not empty. Found: \"\"")
		fmt.Println("Using cached version")
		return cache
	}

	lastModTime = info.ModTime()
	cache = newAppConfig

	return cache
}
