package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BackupJob struct {
	Name      string `json:"name"`
	SourceDir string `json:"source_dir"`
	RepoDir   string `json:"repo_dir"`
}

type AppConfig struct {
	Jobs []BackupJob `json:"jobs"`
}

var (
	scriptsDir      string
	sourceAssetsDir string
	restoreBaseDir  string
	resticBin       string
	pwFile          string
	configFile      string
	config          AppConfig
)

func init() {
	exePath, err := os.Executable()
	if err != nil {
		scriptsDir, _ = os.Getwd()
	} else {
		scriptsDir = filepath.Dir(exePath)
	}

	sourceAssetsDir = filepath.Join(scriptsDir, "source")
	restoreBaseDir = filepath.Join(scriptsDir, "restore")

	// Create directory structure if not yet present
	_ = os.MkdirAll(sourceAssetsDir, 0755)
	_ = os.MkdirAll(restoreBaseDir, 0755)

	// Fallback check if running via 'go run'
	if !fileExists(filepath.Join(sourceAssetsDir, "restic.exe")) {
		if cwd, err := os.Getwd(); err == nil {
			cwdSource := filepath.Join(cwd, "source")
			if fileExists(filepath.Join(cwdSource, "restic.exe")) {
				scriptsDir = cwd
				sourceAssetsDir = cwdSource
				restoreBaseDir = filepath.Join(cwd, "restore")
			}
		}
	}

	resticBin = filepath.Join(sourceAssetsDir, "restic.exe")
	pwFile = filepath.Join(sourceAssetsDir, "password.txt")
	configFile = filepath.Join(sourceAssetsDir, "config.json")

	loadConfig()
}

func loadConfig() {
	if fileExists(configFile) {
		data, err := os.ReadFile(configFile)
		if err == nil {
			_ = json.Unmarshal(data, &config)
		}
	}
}

func saveConfig(cfg AppConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0644)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func pause(reader *bufio.Reader) {
	fmt.Print("\nPress Enter to continue...")
	reader.ReadString('\n')
	fmt.Println()
}

func ensurePassword(reader *bufio.Reader) bool {
	if fileExists(pwFile) {
		return true
	}
	fmt.Println("\nEnter a master password for your encrypted backups.")
	fmt.Println("NOTE: Store this safely! If lost, your backups cannot be recovered.\n")
	fmt.Print("Enter backup password: ")

	rawPw, _ := reader.ReadString('\n')
	pw := strings.TrimRight(rawPw, "\r\n")

	if pw == "" {
		fmt.Println("[ERROR] Password cannot be blank!")
		return false
	}

	if err := os.WriteFile(pwFile, []byte(pw+"\r\n"), 0600); err != nil {
		fmt.Printf("[ERROR] Failed to save password file: %v\n", err)
		return false
	}
	fmt.Printf("[+] Saved password to %s\n\n", pwFile)
	return true
}

func runSetupWizard(reader *bufio.Reader) {
	fmt.Println("\n===================================================")
	fmt.Println("         Restic Multi-Profile Setup Wizard         ")
	fmt.Println("===================================================\n")

	if !fileExists(resticBin) {
		fmt.Printf("[ERROR] restic.exe not found at %s!\n", resticBin)
		fmt.Printf("Please place restic.exe inside: %s\n", sourceAssetsDir)
		return
	}

	if !ensurePassword(reader) {
		return
	}

	if len(config.Jobs) > 0 {
		fmt.Printf("Currently configured profiles (%d):\n", len(config.Jobs))
		for i, j := range config.Jobs {
			fmt.Printf("  [%d] %s: %s -> %s\n", i+1, j.Name, j.SourceDir, j.RepoDir)
		}
		fmt.Println("\nChoose an option:")
		fmt.Println("1. Add new profiles to existing list")
		fmt.Println("2. Overwrite and start from scratch")
		fmt.Println("3. Cancel")
		fmt.Print("Choice [1-3]: ")

		setupChoice, _ := reader.ReadString('\n')
		switch strings.TrimSpace(setupChoice) {
		case "1":
			// Keep existing items
		case "2":
			config.Jobs = []BackupJob{}
		default:
			fmt.Println("Setup cancelled.")
			return
		}
	}

	for {
		fmt.Println("\n--- New Backup Profile ---")

		// 1. Profile Name
		var name string
		for {
			fmt.Print("Enter a friendly name for this profile (e.g. 'Drive D', 'Outlook'): ")
			raw, _ := reader.ReadString('\n')
			name = strings.TrimSpace(raw)
			if name != "" {
				break
			}
			fmt.Println("[ERROR] Profile name cannot be blank.")
		}

		// 2. Source Directory
		var sourceDir string
		for {
			fmt.Print("Enter SOURCE directory to back up (e.g. 'D:\\' or folder path): ")
			raw, _ := reader.ReadString('\n')
			sourceDir = filepath.Clean(strings.TrimSpace(raw))

			if !dirExists(sourceDir) {
				fmt.Printf("[WARNING] Directory '%s' does not exist or is currently unavailable.\n", sourceDir)
				fmt.Print("Do you still want to keep this path? (y/n): ")
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
					break
				}
			} else {
				break
			}
		}

		// 3. Target Repository Directory
		var repoDir string
		for {
			fmt.Print("Enter TARGET repository directory (e.g. 'E:\\restic_backup_d'): ")
			raw, _ := reader.ReadString('\n')
			repoDir = filepath.Clean(strings.TrimSpace(raw))

			vol := filepath.VolumeName(repoDir)
			if vol != "" && !dirExists(vol+"\\") {
				fmt.Printf("[WARNING] Drive '%s\\' is not currently mounted.\n", vol)
				fmt.Print("Keep this repository path anyway? (y/n): ")
				confirm, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(confirm)) == "y" {
					break
				}
			} else {
				break
			}
		}

		job := BackupJob{
			Name:      name,
			SourceDir: sourceDir,
			RepoDir:   repoDir,
		}

		// 4. Initialize Repository
		repoConfig := filepath.Join(job.RepoDir, "config")
		if fileExists(repoConfig) {
			fmt.Printf("[INFO] A Restic repository already exists at %s.\n", job.RepoDir)
		} else {
			fmt.Printf("Initializing repository at %s...\n", job.RepoDir)
			cmd := exec.Command(resticBin, "init", "--repo", job.RepoDir, "--password-file", pwFile)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			if err := cmd.Run(); err != nil {
				fmt.Printf("[ERROR] Repository initialization failed: %v\n", err)
				fmt.Print("Add this profile anyway? (y/n): ")
				keep, _ := reader.ReadString('\n')
				if strings.ToLower(strings.TrimSpace(keep)) != "y" {
					goto AskAnother
				}
			} else {
				fmt.Println("[+] Repository successfully initialized!")
			}
		}

		config.Jobs = append(config.Jobs, job)
		_ = saveConfig(config)
		fmt.Printf("[+] Profile '%s' saved to configuration!\n", job.Name)

	AskAnother:
		fmt.Print("\nWould you like to add another backup profile? (y/n): ")
		again, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(again)) != "y" {
			break
		}
	}

	fmt.Printf("\nSetup completed. All profiles saved in: %s\n", configFile)
}

func runSingleBackup(job BackupJob) bool {
	fmt.Printf("\n--- Running Backup: [%s] ---\n", job.Name)
	fmt.Printf("Source: %s\n", job.SourceDir)
	fmt.Printf("Target: %s\n", job.RepoDir)

	if !fileExists(filepath.Join(job.RepoDir, "config")) {
		fmt.Printf("[ERROR] Repository not found at %s. Please connect the drive or run setup.\n", job.RepoDir)
		return false
	}
	if !dirExists(job.SourceDir) {
		fmt.Printf("[ERROR] Source path '%s' does not exist.\n", job.SourceDir)
		return false
	}

	cmd := exec.Command(resticBin, "-r", job.RepoDir, "--password-file", pwFile, "backup", job.SourceDir, "--use-fs-snapshot")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n[!] Backup failed for [%s]. Check if running as Administrator.\n", job.Name)
		return false
	}

	fmt.Printf("\n[+] Backup completed successfully for [%s]!\n", job.Name)
	return true
}

func runAllBackups() {
	if len(config.Jobs) == 0 {
		fmt.Println("[ERROR] No backup profiles found. Run the setup wizard first.")
		return
	}
	if !fileExists(pwFile) {
		fmt.Println("[ERROR] Password file missing. Run setup first.")
		return
	}

	for _, job := range config.Jobs {
		runSingleBackup(job)
	}
}

func runRestore(reader *bufio.Reader) bool {
	fmt.Println("\n===================================================")
	fmt.Println("              Restic Restore Utility               ")
	fmt.Println("===================================================\n")

	if !fileExists(resticBin) {
		fmt.Printf("[ERROR] restic.exe not found at %s!\n", resticBin)
		return false
	}
	if !fileExists(pwFile) {
		fmt.Printf("[ERROR] Password file not found at %s!\n", pwFile)
		return false
	}
	if len(config.Jobs) == 0 {
		fmt.Println("[ERROR] No profiles found. Run the setup wizard first.")
		return false
	}

	fmt.Println("Select repository to restore from:")
	for idx, job := range config.Jobs {
		fmt.Printf("[%d] %s (Repo: %s)\n", idx+1, job.Name, job.RepoDir)
	}
	fmt.Print("Choose profile number: ")
	numStr, _ := reader.ReadString('\n')
	var selected int
	_, _ = fmt.Sscanf(strings.TrimSpace(numStr), "%d", &selected)

	if selected < 1 || selected > len(config.Jobs) {
		fmt.Println("[ERROR] Invalid selection.")
		return false
	}

	targetJob := config.Jobs[selected-1]
	repoConfig := filepath.Join(targetJob.RepoDir, "config")
	if !fileExists(repoConfig) {
		fmt.Printf("[ERROR] Repository not found at %s! Make sure the drive is connected.\n", targetJob.RepoDir)
		return false
	}

	fmt.Printf("\nListing snapshots for [%s]...\n\n", targetJob.Name)
	listCmd := exec.Command(resticBin, "-r", targetJob.RepoDir, "--password-file", pwFile, "snapshots")
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	if err := listCmd.Run(); err != nil {
		fmt.Println("[ERROR] Failed to list snapshots.")
		return false
	}

	snapID := "latest"
	fmt.Print("\nEnter Snapshot ID to restore (leave blank for 'latest'): ")
	rawInput, _ := reader.ReadString('\n')
	input := strings.TrimSpace(rawInput)
	if input != "" {
		snapID = input
	}

	safeName := strings.ReplaceAll(targetJob.Name, " ", "_")
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	targetDir := filepath.Join(restoreBaseDir, fmt.Sprintf("restored_%s_%s", safeName, timestamp))

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		fmt.Printf("[ERROR] Could not create target directory: %v\n", err)
		return false
	}

	fmt.Printf("\nRestoring snapshot '%s' into:\n%s\n\n", snapID, targetDir)
	restoreCmd := exec.Command(resticBin, "-r", targetJob.RepoDir, "--password-file", pwFile, "restore", snapID, "--target", targetDir)
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr
	restoreCmd.Stdin = os.Stdin

	if err := restoreCmd.Run(); err != nil {
		fmt.Println("\n[!] Restore failed.")
		return false
	}

	fmt.Println("\n[+] Restore completed successfully!")
	fmt.Printf("Files are in: %s\n", targetDir)
	return true
}

func runPrune(reader *bufio.Reader) bool {
	fmt.Println("\n===================================================")
	fmt.Println("             Restic Retention & Prune              ")
	fmt.Println("===================================================\n")

	if !fileExists(resticBin) {
		fmt.Printf("[ERROR] restic.exe not found at %s!\n", resticBin)
		return false
	}
	if !fileExists(pwFile) {
		fmt.Printf("[ERROR] Password file not found at %s!\n", pwFile)
		return false
	}
	if len(config.Jobs) == 0 {
		fmt.Println("[ERROR] No profiles found. Run setup first.")
		return false
	}

	fmt.Println("Select repository to clean:")
	for idx, job := range config.Jobs {
		fmt.Printf("[%d] %s (Repo: %s)\n", idx+1, job.Name, job.RepoDir)
	}
	fmt.Print("Choose profile number: ")
	numStr, _ := reader.ReadString('\n')
	var selected int
	_, _ = fmt.Sscanf(strings.TrimSpace(numStr), "%d", &selected)

	if selected < 1 || selected > len(config.Jobs) {
		fmt.Println("[ERROR] Invalid selection.")
		return false
	}

	targetJob := config.Jobs[selected-1]
	repoConfig := filepath.Join(targetJob.RepoDir, "config")
	if !fileExists(repoConfig) {
		fmt.Printf("[ERROR] Repository not found at %s! Make sure the drive is connected.\n", targetJob.RepoDir)
		return false
	}

	fmt.Printf("\nListing existing snapshots for [%s]...\n\n", targetJob.Name)
	listCmd := exec.Command(resticBin, "-r", targetJob.RepoDir, "--password-file", pwFile, "snapshots")
	listCmd.Stdout = os.Stdout
	listCmd.Stderr = os.Stderr
	_ = listCmd.Run()

	fmt.Println("\nCleanup Options:")
	fmt.Println("1. Keep last N snapshots (delete older ones)")
	fmt.Println("2. Delete a specific snapshot by ID")
	fmt.Println("3. Cancel")
	fmt.Print("Select option [1-3]: ")

	opt, _ := reader.ReadString('\n')
	var pruneCmd *exec.Cmd

	switch strings.TrimSpace(opt) {
	case "1":
		fmt.Print("How many recent snapshots do you want to keep? (e.g. 3, 5): ")
		countStr, _ := reader.ReadString('\n')
		count := strings.TrimSpace(countStr)
		if count == "" {
			fmt.Println("[ERROR] Snapshot count required.")
			return false
		}
		fmt.Printf("\nApplying retention policy (keep-last %s) and repacking repository...\n", count)
		pruneCmd = exec.Command(resticBin, "-r", targetJob.RepoDir, "--password-file", pwFile, "forget", "--keep-last", count, "--prune")

	case "2":
		fmt.Print("Enter Snapshot ID to delete: ")
		snapID, _ := reader.ReadString('\n')
		snapID = strings.TrimSpace(snapID)
		if snapID == "" {
			fmt.Println("[ERROR] Snapshot ID cannot be blank.")
			return false
		}
		fmt.Printf("\nDeleting snapshot '%s' and repacking repository...\n", snapID)
		pruneCmd = exec.Command(resticBin, "-r", targetJob.RepoDir, "--password-file", pwFile, "forget", snapID, "--prune")

	default:
		fmt.Println("Cleanup cancelled.")
		return false
	}

	pruneCmd.Stdout = os.Stdout
	pruneCmd.Stderr = os.Stderr
	pruneCmd.Stdin = os.Stdin

	if err := pruneCmd.Run(); err != nil {
		fmt.Println("\n[!] Prune operation failed.")
		return false
	}

	fmt.Println("\n[+] Space freed and repository repacked successfully!")
	return true
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	// CLI Mode
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "backup-all":
			runAllBackups()
		case "restore":
			runRestore(reader)
		default:
			fmt.Println("Usage: restic_backup.exe [backup-all|restore]")
		}
		return
	}

	// Interactive Menu Mode
	for {
		fmt.Println("===================================================")
		fmt.Println("            Multi-Profile Restic Manager           ")
		fmt.Println("===================================================")
		fmt.Println("1. Setup Wizard (Add/Configure Profiles)")
		fmt.Println("2. Run ALL Backups Sequentially")
		fmt.Println("3. Run Single Backup Task")
		fmt.Println("4. Restore from a Repository")
		fmt.Println("5. Clean Up / Free Disk Space (Forget & Prune)")
		fmt.Println("6. View Configured Profiles")
		fmt.Println("7. Exit")
		fmt.Println("===================================================")
		fmt.Print("Select an option [1-7]: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			runSetupWizard(reader)
			pause(reader)
		case "2":
			runAllBackups()
			pause(reader)
		case "3":
			if len(config.Jobs) == 0 {
				fmt.Println("\nNo profiles found. Run Option 1 first.")
			} else {
				fmt.Println("\nAvailable tasks:")
				for idx, job := range config.Jobs {
					fmt.Printf("[%d] %s (%s -> %s)\n", idx+1, job.Name, job.SourceDir, job.RepoDir)
				}
				fmt.Print("Choose job number: ")
				numStr, _ := reader.ReadString('\n')
				var selected int
				_, _ = fmt.Sscanf(strings.TrimSpace(numStr), "%d", &selected)
				if selected >= 1 && selected <= len(config.Jobs) {
					runSingleBackup(config.Jobs[selected-1])
				} else {
					fmt.Println("Invalid selection.")
				}
			}
			pause(reader)
		case "4":
			runRestore(reader)
			pause(reader)
		case "5":
			runPrune(reader)
			pause(reader)
		case "6":
			fmt.Println("\n--- Configured Profiles ---")
			if len(config.Jobs) == 0 {
				fmt.Println("No profiles configured yet.")
			} else {
				for i, j := range config.Jobs {
					fmt.Printf("[%d] %s\n    Source: %s\n    Target: %s\n", i+1, j.Name, j.SourceDir, j.RepoDir)
				}
			}
			pause(reader)
		case "7":
			return
		default:
			fmt.Println("Invalid selection. Try again.\n")
		}
	}
}
