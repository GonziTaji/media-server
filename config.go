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
		fmt.Printf("Error obteniendo informacion de archivo de configuracion en %s. Error: %s\n", configPath, err.Error())
		fmt.Println("Usando version en memoria")
		return cache
	}

	if info.ModTime().Equal(lastModTime) {
		return cache
	}

	data, err := os.ReadFile(configPath)

	if err != nil {
		fmt.Printf("Error leyendo archivo de configuracion en %s. Error: %s\n", configPath, err.Error())
		fmt.Println("Usando version en memoria")
		return cache
	}

	var newAppConfig AppConfig

	if err := json.Unmarshal(data, &newAppConfig); err != nil {
		fmt.Printf("Configuracion en %s invalida: %s \n", configPath, err.Error())
		fmt.Println("Usando version en memoria")
		return cache
	}

	if newAppConfig.Base64NamePrefix == "" {
		fmt.Println("Error validando configuracion. \"base_64_name_prefix\" vacio. Campo es requerido")
		fmt.Println("Usando version en memoria")
		return cache
	}

	lastModTime = info.ModTime()
	cache = newAppConfig

	return cache
}
