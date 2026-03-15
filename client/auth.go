package main

import (
	"cloud-proxy-pool/config"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
)

const (
	keysFileName = "keys.json"
)

type AuthKey struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Label    string `json:"label"`
}

type KeysStorage struct {
	Keys []AuthKey `json:"keys"`
}

func (ks *KeysStorage) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(ks)
}

func LoadKeysStorage(path string) (*KeysStorage, error) {
	ks := &KeysStorage{Keys: []AuthKey{}}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ks, ks.Save(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(ks); err != nil {
		return nil, err
	}

	return ks, nil
}

func generateRandomPassword(length int) (string, error) {
	if length < 8 {
		length = 16
	}
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(randomBytes)[:length], nil
}

func handleAuthCommand(args []string, configPath string) {
	if len(args) < 1 {
		color.Red("Usage: cloud-proxy-pool auth <command>")
		color.Yellow("Commands: generate, list, delete, select, clear")
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	keysPath := filepath.Join(filepath.Dir(configPath), keysFileName)
	ks, err := LoadKeysStorage(keysPath)
	if err != nil {
		log.Fatalf("failed to load keys storage: %v", err)
	}

	switch cmd {
	case "generate":
		handleGenerate(ks, keysPath, cmdArgs, configPath)
	case "list":
		handleList(ks)
	case "delete":
		handleDelete(ks, keysPath, cmdArgs)
	case "select":
		handleSelect(ks, keysPath, cmdArgs, configPath)
	case "clear":
		handleClear(ks, keysPath, configPath)
	default:
		color.Red("Unknown command: %s", cmd)
		color.Yellow("Available commands: generate, list, delete, select, clear")
		os.Exit(1)
	}
}

func handleGenerate(ks *KeysStorage, keysPath string, args []string, configPath string) {
	if len(args) < 1 {
		username := "user"
		if len(ks.Keys) > 0 {
			username = fmt.Sprintf("user%d", len(ks.Keys)+1)
		}

		password, err := generateRandomPassword(16)
		if err != nil {
			log.Fatalf("failed to generate password: %v", err)
		}

		key := AuthKey{
			Username: username,
			Password: password,
			Label:    fmt.Sprintf("Auto-generated key %d", len(ks.Keys)+1),
		}

		ks.Keys = append(ks.Keys, key)
		if err := ks.Save(keysPath); err != nil {
			log.Fatalf("failed to save keys: %v", err)
		}

		color.Green("Generated new key:")
		fmt.Printf("  Username: %s\n", key.Username)
		fmt.Printf("  Password: %s\n", key.Password)
		fmt.Printf("  Label: %s\n", key.Label)

		if err := selectKey(key.Username, key.Password, configPath); err != nil {
			color.Yellow("Warning: failed to update config: %v", err)
		}
		return
	}

	username := args[0]
	password := ""
	label := ""

	if len(args) >= 2 {
		password = args[1]
	} else {
		var err error
		password, err = generateRandomPassword(16)
		if err != nil {
			log.Fatalf("failed to generate password: %v", err)
		}
	}

	if len(args) >= 3 {
		label = strings.Join(args[2:], " ")
	} else {
		label = fmt.Sprintf("Key for %s", username)
	}

	key := AuthKey{
		Username: username,
		Password: password,
		Label:    label,
	}

	for i, k := range ks.Keys {
		if k.Username == username {
			ks.Keys[i] = key
			color.Yellow("Updated existing key for user: %s", username)
		}
	}

	if len(ks.Keys) == 0 || ks.Keys[len(ks.Keys)-1].Username != username {
		ks.Keys = append(ks.Keys, key)
	}

	if err := ks.Save(keysPath); err != nil {
		log.Fatalf("failed to save keys: %v", err)
	}

	color.Green("Generated key:")
	fmt.Printf("  Username: %s\n", key.Username)
	fmt.Printf("  Password: %s\n", key.Password)
	fmt.Printf("  Label: %s\n", key.Label)

	if err := selectKey(key.Username, key.Password, configPath); err != nil {
		color.Yellow("Warning: failed to update config: %v", err)
	}
}

func handleList(ks *KeysStorage) {
	if len(ks.Keys) == 0 {
		color.Yellow("No keys stored yet. Use 'auth generate' to create one.")
		return
	}

	color.Cyan("Stored keys (%d):", len(ks.Keys))
	for i, key := range ks.Keys {
		fmt.Printf("  [%d] %s - %s\n", i+1, key.Username, key.Label)
	}
}

func handleDelete(ks *KeysStorage, keysPath string, args []string) {
	if len(args) < 1 {
		color.Red("Usage: cloud-proxy-pool auth delete <index|username>")
		os.Exit(1)
	}

	target := args[0]
	idx := -1

	for i, key := range ks.Keys {
		if fmt.Sprintf("%d", i+1) == target || key.Username == target {
			idx = i
			break
		}
	}

	if idx == -1 {
		color.Red("Key not found: %s", target)
		return
	}

	deleted := ks.Keys[idx]
	ks.Keys = append(ks.Keys[:idx], ks.Keys[idx+1:]...)

	if err := ks.Save(keysPath); err != nil {
		log.Fatalf("failed to save keys: %v", err)
	}

	color.Green("Deleted key: %s", deleted.Username)
}

func handleSelect(ks *KeysStorage, keysPath string, args []string, configPath string) {
	if len(args) < 1 {
		color.Red("Usage: cloud-proxy-pool auth select <index|username>")
		os.Exit(1)
	}

	target := args[0]
	var selected *AuthKey

	for i, key := range ks.Keys {
		if fmt.Sprintf("%d", i+1) == target || key.Username == target {
			selected = &key
			break
		}
	}

	if selected == nil {
		color.Red("Key not found: %s", target)
		return
	}

	if err := selectKey(selected.Username, selected.Password, configPath); err != nil {
		log.Fatalf("failed to update config: %v", err)
	}

	color.Green("Selected key: %s", selected.Username)
}

func handleClear(ks *KeysStorage, keysPath string, configPath string) {
	ks.Keys = []AuthKey{}
	if err := ks.Save(keysPath); err != nil {
		log.Fatalf("failed to save keys: %v", err)
	}

	conf, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	conf.Client.User = ""
	conf.Client.Password = ""

	if err := saveConfig(conf, configPath); err != nil {
		log.Fatalf("failed to save config: %v", err)
	}

	color.Green("Cleared all keys and disabled authentication")
}

func selectKey(username, password, configPath string) error {
	conf, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	conf.Client.User = username
	conf.Client.Password = password

	return saveConfig(conf, configPath)
}

func saveConfig(conf *config.Config, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(conf)
}
