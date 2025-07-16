package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Game represents a single match with all its statistics
type Game struct {
	ID           string         `json:"id"`
	TotalKills   int            `json:"total_kills"`
	Players      []string       `json:"players"`
	Kills        map[string]int `json:"kills"`
	KillsByMeans map[string]int `json:"kills_by_means,omitempty"`
}

// PlayerManager handles consistent player name mapping and normalization
type PlayerManager struct {
	clientToName   map[int]string    // client_id -> canonical_name
	nameVariations map[string]string // variant_name -> canonical_name
	canonicalNames map[string]bool   // track all canonical names
}

// Parser handles the log parsing logic and maintains game state
type Parser struct {
	Games       map[string]*Game // All parsed games indexed by game ID
	CurrentGame *Game            // Currently active game being parsed
	PlayerMgr   *PlayerManager   // Enhanced player name management
	GameCounter int              // Counter for generating game IDs
}

// LogEntry represents a parsed log line with timestamp and event data
type LogEntry struct {
	Timestamp string
	Event     string
	Data      string
}

// PlayerRanking represents a player's total statistics across all games
type PlayerRanking struct {
	Name  string
	Kills int
}

// Regular expressions for parsing different log entry types
var (
	// Matches log lines: "timestamp event_type: event_data"
	logLineRegex = regexp.MustCompile(`^\s*(\d+:\d+)\s+([^:]+):\s*(.*)$`)

	// Matches kill events: "killer_id victim_id weapon_id: killer_name killed victim_name by weapon_name"
	// Updated to handle names with special characters more reliably
	killRegex = regexp.MustCompile(`^(\d+)\s+(\d+)\s+(\d+):\s*(.+?)\s+killed\s+(.+?)\s+by\s+(.+)$`)

	// Matches client info changes to extract player names: "client_id n\PlayerName\..."
	clientInfoRegex = regexp.MustCompile(`^(\d+)\s+n\\([^\\]+)\\`)
)

// NewPlayerManager creates a new player manager instance
func NewPlayerManager() *PlayerManager {
	return &PlayerManager{
		clientToName:   make(map[int]string),
		nameVariations: make(map[string]string),
		canonicalNames: make(map[string]bool),
	}
}

// normalizePlayerName applies consistent normalization rules to player names
func (pm *PlayerManager) normalizePlayerName(name string) string {
	// Trim whitespace
	normalized := strings.TrimSpace(name)

	// Remove trailing punctuation (!, ?, ., etc.)
	for len(normalized) > 0 {
		lastChar := normalized[len(normalized)-1]
		if lastChar == '!' || lastChar == '?' || lastChar == '.' || lastChar == ',' {
			normalized = normalized[:len(normalized)-1]
		} else {
			break
		}
	}

	// Trim again after removing punctuation
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// registerPlayer registers a player with their client ID and establishes canonical name
func (pm *PlayerManager) registerPlayer(clientID int, rawName string) string {
	canonical := pm.normalizePlayerName(rawName)

	// If this client ID already has a canonical name, check if it's consistent
	if existingCanonical, exists := pm.clientToName[clientID]; exists {
		// If the normalized names are the same, keep the existing canonical name
		if canonical == existingCanonical {
			// Still register the raw name variant for lookup
			pm.nameVariations[rawName] = existingCanonical
			return existingCanonical
		}
		// If they're different, the player might have changed their name
		// We'll use the most recent one but log this for debugging
		fmt.Printf("Debug: Player with client ID %d changed name from '%s' to '%s'\n",
			clientID, existingCanonical, canonical)
	}

	// Register the canonical name
	pm.clientToName[clientID] = canonical
	pm.canonicalNames[canonical] = true
	pm.nameVariations[rawName] = canonical

	// Also register the canonical name to itself for consistency
	if canonical != rawName {
		pm.nameVariations[canonical] = canonical
	}

	return canonical
}

// getPlayerName returns the canonical name for a client ID or raw name
func (pm *PlayerManager) getPlayerName(clientID int, fallbackName string) string {
	// First try to get by client ID (most reliable)
	if canonical, exists := pm.clientToName[clientID]; exists {
		return canonical
	}

	// If no client ID mapping, try name variations lookup
	if canonical, exists := pm.nameVariations[fallbackName]; exists {
		return canonical
	}

	// Last resort: normalize the fallback name and register it
	canonical := pm.normalizePlayerName(fallbackName)

	// Register this mapping for future use
	pm.clientToName[clientID] = canonical
	pm.canonicalNames[canonical] = true
	pm.nameVariations[fallbackName] = canonical
	if canonical != fallbackName {
		pm.nameVariations[canonical] = canonical
	}

	return canonical
}

// NewParser creates a new parser instance with initialized state
func NewParser() *Parser {
	return &Parser{
		Games:       make(map[string]*Game),
		PlayerMgr:   NewPlayerManager(),
		GameCounter: 0,
	}
}

// ParseFile reads and processes the entire log file line by line
func (p *Parser) ParseFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Skip empty lines and lines with just dashes (separators)
		if strings.TrimSpace(line) == "" || strings.Contains(line, "----") {
			continue
		}

		// Parse the log entry and handle any parsing errors gracefully
		entry, err := p.parseLine(line)
		if err != nil {
			// Log parsing errors but continue processing (don't fail on malformed lines)
			fmt.Printf("Warning: Could not parse line %d: %s\n", lineNumber, err)
			continue
		}

		// Process the parsed entry based on its event type
		if err := p.processEntry(entry); err != nil {
			fmt.Printf("Warning: Error processing line %d: %s\n", lineNumber, err)
		}
	}

	// Finalize the last game if it exists
	if p.CurrentGame != nil {
		p.finalizeGame()
	}

	return scanner.Err()
}

// parseLine extracts timestamp, event type, and data from a log line
func (p *Parser) parseLine(line string) (*LogEntry, error) {
	matches := logLineRegex.FindStringSubmatch(line)
	if len(matches) != 4 {
		return nil, fmt.Errorf("invalid log line format: %s", line)
	}

	return &LogEntry{
		Timestamp: matches[1],
		Event:     strings.TrimSpace(matches[2]),
		Data:      strings.TrimSpace(matches[3]),
	}, nil
}

// processEntry handles different types of log events and updates game state accordingly
func (p *Parser) processEntry(entry *LogEntry) error {
	switch entry.Event {
	case "InitGame":
		// Start a new game session
		return p.handleInitGame(entry)
	case "Exit":
		// End current game session
		return p.handleExit(entry)
	case "ClientConnect":
		// Player connects to server (we track this but main info comes from ClientUserinfoChanged)
		return p.handleClientConnect(entry)
	case "ClientUserinfoChanged":
		// Extract player name from client info - this is where we get actual player names
		return p.handleClientUserinfoChanged(entry)
	case "Kill":
		// Process kill events - the core logic for tracking kills
		return p.handleKill(entry)
	case "ShutdownGame":
		// Game shutdown event
		return p.handleShutdown(entry)
	default:
		// Ignore other event types (ClientBegin, item pickups, etc.)
		return nil
	}
}

// handleInitGame starts a new game session
func (p *Parser) handleInitGame(entry *LogEntry) error {
	// Finalize previous game if it exists
	if p.CurrentGame != nil {
		p.finalizeGame()
	}

	// Create new game with incremented counter
	p.GameCounter++
	gameID := fmt.Sprintf("game_%d", p.GameCounter)

	p.CurrentGame = &Game{
		ID:           gameID,
		TotalKills:   0,
		Players:      []string{},
		Kills:        make(map[string]int),
		KillsByMeans: make(map[string]int),
	}

	// Clear client-player mapping for new game (players might rejoin with different IDs)
	// Reset the player manager for the new game session
	p.PlayerMgr = NewPlayerManager()

	return nil
}

// handleExit finalizes the current game when it ends
func (p *Parser) handleExit(entry *LogEntry) error {
	if p.CurrentGame != nil {
		p.finalizeGame()
		p.CurrentGame = nil
	}
	return nil
}

// handleClientConnect tracks when a client connects (ID assignment)
func (p *Parser) handleClientConnect(entry *LogEntry) error {
	// We mainly track this for completeness; actual player names come from ClientUserinfoChanged
	return nil
}

// handleClientUserinfoChanged extracts player names from client info updates
func (p *Parser) handleClientUserinfoChanged(entry *LogEntry) error {
	// Parse client ID and player name from the info string
	// Format: "client_id n\PlayerName\t\0\model\..."
	matches := clientInfoRegex.FindStringSubmatch(entry.Data)
	if len(matches) != 3 {
		return fmt.Errorf("could not parse client info: %s", entry.Data)
	}

	clientID, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("invalid client ID: %s", matches[1])
	}

	rawPlayerName := matches[2]

	// Register player with the player manager to get canonical name
	canonicalName := p.PlayerMgr.registerPlayer(clientID, rawPlayerName)

	// Add player to current game if not already present and game exists
	if p.CurrentGame != nil {
		p.addPlayerToGame(canonicalName)
	}

	return nil
}

// handleKill processes kill events and updates kill statistics
func (p *Parser) handleKill(entry *LogEntry) error {
	if p.CurrentGame == nil {
		// If no current game, create one (defensive programming)
		p.handleInitGame(&LogEntry{})
	}

	// Parse kill event: "killer_id victim_id weapon_id: killer_name killed victim_name by weapon_name"
	matches := killRegex.FindStringSubmatch(entry.Data)
	if len(matches) != 7 {
		return fmt.Errorf("could not parse kill event: %s", entry.Data)
	}

	// Extract client IDs
	killerID, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("invalid killer ID: %s", matches[1])
	}

	victimID, err := strconv.Atoi(matches[2])
	if err != nil {
		return fmt.Errorf("invalid victim ID: %s", matches[2])
	}

	// Extract names from the kill event (as fallback)
	killerNameFromEvent := strings.TrimSpace(matches[4])
	victimNameFromEvent := strings.TrimSpace(matches[5])
	weaponName := strings.TrimSpace(matches[6])

	// Get canonical player names using enhanced player management
	// This will use client ID mapping when available, or normalize the name as fallback
	var killerName, victimName string

	if killerNameFromEvent == "<world>" {
		killerName = "<world>"
	} else {
		killerName = p.PlayerMgr.getPlayerName(killerID, killerNameFromEvent)
	}

	victimName = p.PlayerMgr.getPlayerName(victimID, victimNameFromEvent)

	// Debug output to track name mapping
	if killerNameFromEvent != "<world>" && killerNameFromEvent != killerName {
		fmt.Printf("Debug: Killer name mapped from '%s' to '%s'\n", killerNameFromEvent, killerName)
	}
	if victimNameFromEvent != victimName {
		fmt.Printf("Debug: Victim name mapped from '%s' to '%s'\n", victimNameFromEvent, victimName)
	}

	// Increment total kills for the game
	p.CurrentGame.TotalKills++

	// Track weapon/means of death statistics
	p.CurrentGame.KillsByMeans[weaponName]++

	// Add both killer and victim to players list (victim always gets added)
	p.addPlayerToGame(victimName)

	// Handle kill scoring based on business rules
	if killerName == "<world>" {
		// World kills: victim loses a kill (minimum 0)
		// <world> is not considered a player and doesn't get added to players list
		if p.CurrentGame.Kills[victimName] > 0 {
			p.CurrentGame.Kills[victimName]--
		}
	} else {
		// Regular player kill: add killer to game and increment their kill count
		p.addPlayerToGame(killerName)
		p.CurrentGame.Kills[killerName]++
	}

	return nil
}

// handleShutdown processes game shutdown events
func (p *Parser) handleShutdown(entry *LogEntry) error {
	// Similar to Exit - finalize current game
	if p.CurrentGame != nil {
		p.finalizeGame()
		p.CurrentGame = nil
	}
	return nil
}

// addPlayerToGame adds a player to the current game if not already present
func (p *Parser) addPlayerToGame(playerName string) {
	if playerName == "<world>" {
		return // <world> is never added as a player
	}

	// Check if player already exists in the game
	for _, existingPlayer := range p.CurrentGame.Players {
		if existingPlayer == playerName {
			return // Player already in game
		}
	}

	// Add new player to the game
	p.CurrentGame.Players = append(p.CurrentGame.Players, playerName)

	// Initialize kill count if not exists
	if _, exists := p.CurrentGame.Kills[playerName]; !exists {
		p.CurrentGame.Kills[playerName] = 0
	}
}

// finalizeGame completes the current game and adds it to the games map
func (p *Parser) finalizeGame() {
	if p.CurrentGame == nil {
		return
	}

	// Sort players list for consistent output
	sort.Strings(p.CurrentGame.Players)

	// Store the completed game
	p.Games[p.CurrentGame.ID] = p.CurrentGame
}

// GetSingleGameOutput returns the basic required output format for single game analysis
func (p *Parser) GetSingleGameOutput() map[string]interface{} {
	// Aggregate all games into a single output (for basic requirement)
	allPlayers := make(map[string]bool)
	totalKills := make(map[string]int)

	// Combine statistics from all games
	for _, game := range p.Games {
		for _, player := range game.Players {
			allPlayers[player] = true
			totalKills[player] += game.Kills[player]
		}
	}

	// Convert players map to sorted slice
	players := make([]string, 0, len(allPlayers))
	for player := range allPlayers {
		players = append(players, player)
	}
	sort.Strings(players)

	return map[string]interface{}{
		"players": players,
		"kills":   totalKills,
	}
}

// GetMultiGameOutput returns the advanced output format organized by game
func (p *Parser) GetMultiGameOutput() map[string]*Game {
	return p.Games
}

// GetPlayerRankings returns player rankings across all games
func (p *Parser) GetPlayerRankings() []PlayerRanking {
	playerTotals := make(map[string]int)

	// Aggregate kills across all games
	for _, game := range p.Games {
		for player, kills := range game.Kills {
			playerTotals[player] += kills
		}
	}

	// Convert to slice and sort by kills (descending)
	rankings := make([]PlayerRanking, 0, len(playerTotals))
	for player, kills := range playerTotals {
		rankings = append(rankings, PlayerRanking{Name: player, Kills: kills})
	}

	// Sort by kills descending, then by name ascending for tie-breaking
	sort.Slice(rankings, func(i, j int) bool {
		if rankings[i].Kills == rankings[j].Kills {
			return rankings[i].Name < rankings[j].Name
		}
		return rankings[i].Kills > rankings[j].Kills
	})

	return rankings
}

// PrintRankings outputs formatted player rankings
func (p *Parser) PrintRankings() {
	rankings := p.GetPlayerRankings()
	fmt.Println("\nPlayer Rankings:")
	fmt.Println("================")

	for i, ranking := range rankings {
		fmt.Printf("%d. %s - %d kills\n", i+1, ranking.Name, ranking.Kills)
	}
}

func main() {
	// Check command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <log_file> [output_format]")
		fmt.Println("Output formats: basic (default), multi, ranking")
		os.Exit(1)
	}

	logFile := os.Args[1]
	outputFormat := "basic"
	if len(os.Args) > 2 {
		outputFormat = os.Args[2]
	}

	// Create parser and process the log file
	parser := NewParser()

	fmt.Printf("Parsing log file: %s\n", logFile)
	if err := parser.ParseFile(logFile); err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	fmt.Printf("Successfully parsed %d games\n", len(parser.Games))

	// Output results based on requested format
	switch outputFormat {
	case "basic":
		// Basic required output format
		output := parser.GetSingleGameOutput()
		jsonOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			log.Fatalf("Error marshaling JSON: %v", err)
		}
		fmt.Println("\nBasic Output:")
		fmt.Println(string(jsonOutput))

	case "multi":
		// Advanced multi-game output format
		output := parser.GetMultiGameOutput()
		jsonOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			log.Fatalf("Error marshaling JSON: %v", err)
		}
		fmt.Println("\nMulti-Game Output:")
		fmt.Println(string(jsonOutput))

	case "ranking":
		// Player rankings output
		parser.PrintRankings()

	case "all":
		// Output all formats
		fmt.Println("\n=== BASIC OUTPUT ===")
		basic := parser.GetSingleGameOutput()
		basicJSON, _ := json.MarshalIndent(basic, "", "  ")
		fmt.Println(string(basicJSON))

		fmt.Println("\n=== MULTI-GAME OUTPUT ===")
		multi := parser.GetMultiGameOutput()
		multiJSON, _ := json.MarshalIndent(multi, "", "  ")
		fmt.Println(string(multiJSON))

		fmt.Println("\n=== PLAYER RANKINGS ===")
		parser.PrintRankings()

	default:
		fmt.Printf("Unknown output format: %s\n", outputFormat)
		fmt.Println("Available formats: basic, multi, ranking, all")
		os.Exit(1)
	}
}
