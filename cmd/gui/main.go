package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type LogMessage struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"` // "mkfs", "mount", "system"
	Text      string `json:"text"`
}

var (
	logs       []LogMessage
	logsMu     sync.Mutex
	clients    = make(map[chan LogMessage]bool)
	clientsMu  sync.Mutex
	mountCmd   *exec.Cmd
	mountCmdMu sync.Mutex
	mountPoint string
	mountDev   string
)

func addLog(source, text string) {
	msg := LogMessage{
		Timestamp: time.Now().Format("15:04:05"),
		Source:    source,
		Text:      strings.TrimSpace(text),
	}
	logsMu.Lock()
	logs = append(logs, msg)
	if len(logs) > 500 {
		logs = logs[1:]
	}
	logsMu.Unlock()

	clientsMu.Lock()
	for ch := range clients {
		select {
		case ch <- msg:
		default:
		}
	}
	clientsMu.Unlock()
}

func getLogs() []LogMessage {
	logsMu.Lock()
	defer logsMu.Unlock()
	copied := make([]LogMessage, len(logs))
	copy(copied, logs)
	return copied
}

func main() {
	port := flag.Int("port", 8080, "Port to run the GUI server on")
	flag.Parse()

	http.HandleFunc("/api/disks", handleGetDisks)
	http.HandleFunc("/api/format", handleFormat)
	http.HandleFunc("/api/mount", handleMount)
	http.HandleFunc("/api/unmount", handleUnmount)
	http.HandleFunc("/api/status", handleStatus)
	http.HandleFunc("/api/logs/stream", handleLogStream)
	http.HandleFunc("/api/logs/history", handleLogHistory)

	// Serve built-in static frontend assets
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(htmlContent))
			return
		}
		http.NotFound(w, r)
	})

	addLog("system", "TEXF GUI Server started on port "+fmt.Sprint(*port))
	addLog("system", "Open http://localhost:"+fmt.Sprint(*port)+" in your browser")

	// Print message to console
	fmt.Printf("TEXF GUI Server running at http://localhost:%d/\n", *port)

	// Start browser if possible (ignore error)
	go startBrowser("http://localhost:" + fmt.Sprint(*port))

	err := http.ListenAndServe(":"+fmt.Sprint(*port), nil)
	if err != nil {
		fmt.Printf("Server failed: %v\n", err)
	}
}

type DiskInfo struct {
	Device string `json:"device"`
	Size   string `json:"size"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

func handleGetDisks(w http.ResponseWriter, r *http.Request) {
	var disks []DiskInfo

	if runtime.GOOS == "darwin" {
		// Run diskutil list
		cmd := exec.Command("diskutil", "list")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			scanner := bufio.NewScanner(&out)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "/dev/disk") {
					parts := strings.Fields(line)
					if len(parts) > 0 {
						dev := parts[0]
						// Fetch detail
						size := "Unknown"
						infoCmd := exec.Command("diskutil", "info", dev)
						var infoOut bytes.Buffer
						infoCmd.Stdout = &infoOut
						if errInfo := infoCmd.Run(); errInfo == nil {
							infoScanner := bufio.NewScanner(&infoOut)
							for infoScanner.Scan() {
								infoLine := infoScanner.Text()
								if strings.Contains(infoLine, "Disk Size:") || strings.Contains(infoLine, "Total Size:") {
									sizeParts := strings.Split(infoLine, ":")
									if len(sizeParts) > 1 {
										size = strings.TrimSpace(sizeParts[1])
									}
								}
							}
						}
						disks = append(disks, DiskInfo{
							Device: dev,
							Size:   size,
							Name:   dev,
							Type:   "Disk",
						})
					}
				}
			}
		}
	} else if runtime.GOOS == "linux" {
		// Run lsblk
		cmd := exec.Command("lsblk", "-d", "-o", "NAME,SIZE,TYPE,TRAN")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			scanner := bufio.NewScanner(&out)
			// Skip header
			if scanner.Scan() {
				for scanner.Scan() {
					parts := strings.Fields(scanner.Text())
					if len(parts) >= 2 {
						name := "/dev/" + parts[0]
						size := parts[1]
						t := parts[2]
						disks = append(disks, DiskInfo{
							Device: name,
							Size:   size,
							Name:   name,
							Type:   t,
						})
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(disks)
}

func handleFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Device string `json:"device"`
		Label  string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Device == "" {
		http.Error(w, "Device is required", http.StatusBadRequest)
		return
	}

	addLog("mkfs", "Formatting "+req.Device+" (Label: "+req.Label+")...")

	args := []string{}
	if req.Label != "" {
		args = append(args, "-label", req.Label)
	}
	args = append(args, req.Device)

	// Run mkfs.texf. Use sudo if needed.
	// Since user is running via sudo/root, it should execute directly
	cmdPath := "./mkfs.texf"
	if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
		addLog("mkfs", "Error: mkfs.texf binary not found in current directory. Please run 'make' first.")
		http.Error(w, "mkfs.texf not built", http.StatusInternalServerError)
		return
	}

	cmd := exec.Command(cmdPath, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		addLog("mkfs", "Failed to start mkfs.texf: "+err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go logReader("mkfs", stdout)
	go logReader("mkfs", stderr)

	go func() {
		err := cmd.Wait()
		if err == nil {
			addLog("mkfs", "Success: Formatting completed.")
		} else {
			addLog("mkfs", "Failed: Formatting failed with error: "+err.Error())
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"started"}`))
}

func handleMount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Device     string `json:"device"`
		MountPoint string `json:"mountpoint"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Device == "" || req.MountPoint == "" {
		http.Error(w, "Device and Mount Point are required", http.StatusBadRequest)
		return
	}

	mountCmdMu.Lock()
	if mountCmd != nil {
		mountCmdMu.Unlock()
		http.Error(w, "A mount operation is already active", http.StatusConflict)
		return
	}
	mountCmdMu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(req.MountPoint, 0755); err != nil {
		addLog("mount", "Failed to create mount directory: "+err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cmdPath := "./texf-mount"
	if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
		addLog("mount", "Error: texf-mount binary not found. Please install FUSE (libfuse-dev on Linux, or macFUSE/FUSE-T on macOS) and run 'make' to enable mount support.")
		http.Error(w, "texf-mount not built", http.StatusInternalServerError)
		return
	}

	addLog("mount", "Mounting "+req.Device+" at "+req.MountPoint+"...")

	// Pass foreground flag and allow_other
	args := []string{req.Device, req.MountPoint, "-f", "-o", "allow_other"}
	cmd := exec.Command(cmdPath, args...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		addLog("mount", "Failed to start mount process: "+err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	mountCmdMu.Lock()
	mountCmd = cmd
	mountPoint = req.MountPoint
	mountDev = req.Device
	mountCmdMu.Unlock()

	go logReader("mount", stdout)
	go logReader("mount", stderr)

	go func() {
		err := cmd.Wait()
		mountCmdMu.Lock()
		mountCmd = nil
		p := mountPoint
		mountPoint = ""
		mountDev = ""
		mountCmdMu.Unlock()

		if err == nil {
			addLog("mount", "Mount stopped cleanly.")
		} else {
			addLog("mount", "Mount process exited: "+err.Error())
			// Unmount just in case
			unmount(p)
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"mounted"}`))
}

func handleUnmount(w http.ResponseWriter, r *http.Request) {
	mountCmdMu.Lock()
	p := mountPoint
	cmd := mountCmd
	mountCmdMu.Unlock()

	if p == "" {
		// Fallback: try unmounting `./mnt` default if running
		p = "./mnt"
	}

	addLog("mount", "Unmounting "+p+"...")

	if cmd != nil {
		cmd.Process.Kill()
	}

	err := unmount(p)
	if err == nil {
		addLog("mount", "Success: Unmounted "+p)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"unmounted"}`))
	} else {
		addLog("mount", "Error unmounting: "+err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func unmount(p string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("umount", p)
	} else {
		cmd = exec.Command("fusermount", "-u", p)
	}
	return cmd.Run()
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	mountCmdMu.Lock()
	active := mountCmd != nil
	p := mountPoint
	dev := mountDev
	mountCmdMu.Unlock()

	status := map[string]interface{}{
		"active":      active,
		"mountpoint":  p,
		"device":      dev,
		"platform":    runtime.GOOS,
		"architecture": runtime.GOARCH,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleLogHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getLogs())
}

func handleLogStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan LogMessage, 20)
	clientsMu.Lock()
	clients[ch] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, ch)
		clientsMu.Unlock()
		close(ch)
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
		}
	}
}

func logReader(source string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		addLog(source, scanner.Text())
	}
}

func startBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Run()
}

const htmlContent = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>TEXF Filesystem Manager</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Outfit:wght@400;500;600;700&family=Fira+Code:wght@400;500&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0d0e12;
            --panel-bg: rgba(22, 24, 33, 0.7);
            --border-color: rgba(255, 255, 255, 0.08);
            --primary: #8a2be2;
            --primary-glow: rgba(138, 43, 226, 0.4);
            --accent: #00f0ff;
            --accent-glow: rgba(0, 240, 255, 0.3);
            --text-main: #f3f4f6;
            --text-muted: #9ca3af;
            --success: #10b981;
            --error: #ef4444;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Inter', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-main);
            height: 100vh;
            display: flex;
            flex-direction: column;
            overflow: hidden;
            background-image: 
                radial-gradient(circle at 10% 20%, rgba(138, 43, 226, 0.15) 0%, transparent 40%),
                radial-gradient(circle at 90% 80%, rgba(0, 240, 255, 0.1) 0%, transparent 40%);
        }

        header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 1.5rem 2rem;
            border-bottom: 1px solid var(--border-color);
            background: rgba(13, 14, 18, 0.8);
            backdrop-filter: blur(12px);
            z-index: 10;
        }

        h1 {
            font-family: 'Outfit', sans-serif;
            font-size: 1.8rem;
            font-weight: 700;
            background: linear-gradient(135deg, var(--text-main) 30%, var(--primary) 70%, var(--accent));
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .badge {
            font-size: 0.75rem;
            padding: 0.2rem 0.6rem;
            border-radius: 9999px;
            background: rgba(138, 43, 226, 0.2);
            color: #d8b4fe;
            border: 1px solid rgba(138, 43, 226, 0.4);
            font-weight: 600;
            letter-spacing: 0.05em;
        }

        .sys-info {
            display: flex;
            gap: 1.5rem;
            font-size: 0.85rem;
            color: var(--text-muted);
        }

        .sys-info span {
            display: flex;
            align-items: center;
            gap: 0.4rem;
        }

        .container {
            display: grid;
            grid-template-columns: 380px 1fr;
            flex: 1;
            overflow: hidden;
            padding: 1.5rem;
            gap: 1.5rem;
        }

        .sidebar {
            display: flex;
            flex-direction: column;
            gap: 1.5rem;
            overflow-y: auto;
        }

        .card {
            background: var(--panel-bg);
            border: 1px solid var(--border-color);
            border-radius: 16px;
            padding: 1.5rem;
            backdrop-filter: blur(16px);
            box-shadow: 0 8px 32px 0 rgba(0, 0, 0, 0.37);
            transition: border-color 0.3s;
        }

        .card:hover {
            border-color: rgba(138, 43, 226, 0.25);
        }

        .card-title {
            font-family: 'Outfit', sans-serif;
            font-size: 1.2rem;
            font-weight: 600;
            margin-bottom: 1.2rem;
            color: var(--text-main);
            display: flex;
            align-items: center;
            gap: 0.5rem;
            border-bottom: 1px solid rgba(255, 255, 255, 0.05);
            padding-bottom: 0.5rem;
        }

        .form-group {
            margin-bottom: 1rem;
        }

        .form-group label {
            display: block;
            font-size: 0.8rem;
            color: var(--text-muted);
            margin-bottom: 0.4rem;
            font-weight: 500;
        }

        .form-control {
            width: 100%;
            background: rgba(0, 0, 0, 0.3);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0.75rem;
            color: var(--text-main);
            font-family: inherit;
            font-size: 0.9rem;
            transition: all 0.3s;
        }

        .form-control:focus {
            outline: none;
            border-color: var(--primary);
            box-shadow: 0 0 0 2px var(--primary-glow);
        }

        select.form-control {
            cursor: pointer;
        }

        .btn {
            width: 100%;
            padding: 0.75rem;
            border: none;
            border-radius: 8px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s;
            font-size: 0.9rem;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 0.5rem;
        }

        .btn-primary {
            background: linear-gradient(135deg, #8a2be2, #4b0082);
            color: #fff;
            box-shadow: 0 4px 14px 0 var(--primary-glow);
        }

        .btn-primary:hover {
            opacity: 0.9;
            transform: translateY(-1px);
            box-shadow: 0 6px 20px 0 var(--primary-glow);
        }

        .btn-secondary {
            background: linear-gradient(135deg, #00f0ff, #008b8b);
            color: #0d0e12;
            box-shadow: 0 4px 14px 0 var(--accent-glow);
        }

        .btn-secondary:hover {
            opacity: 0.9;
            transform: translateY(-1px);
            box-shadow: 0 6px 20px 0 var(--accent-glow);
        }

        .btn-danger {
            background: linear-gradient(135deg, #ef4444, #991b1b);
            color: #fff;
            box-shadow: 0 4px 14px 0 rgba(239, 68, 68, 0.3);
        }

        .btn-danger:hover {
            opacity: 0.9;
            transform: translateY(-1px);
            box-shadow: 0 6px 20px 0 rgba(239, 68, 68, 0.4);
        }

        .btn:active {
            transform: translateY(1px);
        }

        .console-container {
            display: flex;
            flex-direction: column;
            background: rgba(10, 11, 15, 0.9);
            border: 1px solid var(--border-color);
            border-radius: 16px;
            overflow: hidden;
            box-shadow: inset 0 2px 8px rgba(0, 0, 0, 0.8);
        }

        .console-header {
            background: rgba(22, 24, 33, 0.8);
            border-bottom: 1px solid var(--border-color);
            padding: 0.8rem 1.5rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .console-tab {
            font-size: 0.85rem;
            font-weight: 600;
            color: var(--text-main);
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .console-actions {
            display: flex;
            gap: 0.8rem;
        }

        .dot-glow {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background-color: var(--success);
            box-shadow: 0 0 8px var(--success);
        }

        .dot-glow.inactive {
            background-color: var(--error);
            box-shadow: 0 0 8px var(--error);
        }

        .clear-btn {
            background: transparent;
            border: none;
            color: var(--text-muted);
            cursor: pointer;
            font-size: 0.8rem;
            transition: color 0.2s;
        }

        .clear-btn:hover {
            color: var(--text-main);
        }

        .console-body {
            flex: 1;
            padding: 1.5rem;
            overflow-y: auto;
            font-family: 'Fira Code', monospace;
            font-size: 0.85rem;
            line-height: 1.5;
            display: flex;
            flex-direction: column;
            gap: 0.4rem;
        }

        .log-entry {
            display: flex;
            gap: 1rem;
            align-items: flex-start;
        }

        .log-time {
            color: #6b7280;
            flex-shrink: 0;
            user-select: none;
        }

        .log-source {
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.75rem;
            padding: 0.1rem 0.4rem;
            border-radius: 4px;
            flex-shrink: 0;
            width: 70px;
            text-align: center;
            user-select: none;
        }

        .source-mkfs {
            background: rgba(168, 85, 247, 0.2);
            color: #d8b4fe;
            border: 1px solid rgba(168, 85, 247, 0.4);
        }

        .source-mount {
            background: rgba(59, 130, 246, 0.2);
            color: #93c5fd;
            border: 1px solid rgba(59, 130, 246, 0.4);
        }

        .source-system {
            background: rgba(245, 158, 11, 0.2);
            color: #fde047;
            border: 1px solid rgba(245, 158, 11, 0.4);
        }

        .log-text {
            color: #d1d5db;
            word-break: break-all;
            white-space: pre-wrap;
        }

        .status-banner {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 1rem 1.5rem;
            background: rgba(16, 185, 129, 0.1);
            border: 1px solid rgba(16, 185, 129, 0.3);
            border-radius: 12px;
            margin-bottom: 1rem;
            display: none;
        }

        .status-banner.error {
            background: rgba(239, 68, 68, 0.1);
            border: 1px solid rgba(239, 68, 68, 0.3);
        }

        .status-info {
            display: flex;
            align-items: center;
            gap: 0.8rem;
        }

        .status-label {
            font-weight: 600;
            font-size: 0.9rem;
        }

        .status-subtext {
            font-size: 0.8rem;
            color: var(--text-muted);
        }

        .refresh-btn {
            background: none;
            border: none;
            color: var(--accent);
            font-size: 0.8rem;
            cursor: pointer;
            margin-left: 0.5rem;
            font-weight: 500;
        }

        .refresh-btn:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <header>
        <div>
            <h1>TEXF File Manager <span class="badge">V1</span></h1>
        </div>
        <div class="sys-info">
            <span id="platform-info">OS: Detecting...</span>
            <span id="arch-info">Arch: Detecting...</span>
        </div>
    </header>

    <div class="container">
        <div class="sidebar">
            <div class="card">
                <div class="card-title">
                    <span>⚙️ Format Storage</span>
                </div>
                <div class="form-group">
                    <label for="format-device">Select Device / Image <button class="refresh-btn" onclick="loadDisks()">⟳ Refresh</button></label>
                    <select id="format-device" class="form-control">
                        <option value="">-- Choose target --</option>
                    </select>
                </div>
                <div class="form-group">
                    <label for="format-custom">Or Custom Device Path / File</label>
                    <input type="text" id="format-custom" class="form-control" placeholder="e.g. /dev/disk4 or verify_disk.img">
                </div>
                <div class="form-group">
                    <label for="format-label">Volume Label</label>
                    <input type="text" id="format-label" class="form-control" value="TEXF_VOLUME" placeholder="e.g. MY_DRIVE">
                </div>
                <button class="btn btn-primary" onclick="formatDevice()">🚀 Format Drive</button>
            </div>

            <div class="card">
                <div class="card-title">
                    <span>🔌 Mount File System</span>
                </div>
                <div class="form-group">
                    <label for="mount-device">Device / Image</label>
                    <input type="text" id="mount-device" class="form-control" placeholder="e.g. /dev/disk4 or verify_disk.img">
                </div>
                <div class="form-group">
                    <label for="mount-point">Mount Point Directory</label>
                    <input type="text" id="mount-point" class="form-control" value="./mnt" placeholder="e.g. ./mnt">
                </div>
                <div class="form-group" style="display: flex; gap: 0.5rem; margin-top: 1.5rem;">
                    <button id="mount-btn" class="btn btn-secondary" onclick="mountDevice()">Mount</button>
                    <button id="unmount-btn" class="btn btn-danger" style="display: none;" onclick="unmountDevice()">Unmount</button>
                </div>
            </div>
        </div>

        <div class="console-container">
            <div class="console-header">
                <div class="console-tab">
                    <div id="status-glow" class="dot-glow inactive"></div>
                    <span>Daemon Logs</span>
                </div>
                <div class="console-actions">
                    <button class="clear-btn" onclick="clearConsole()">Clear Screen</button>
                </div>
            </div>
            
            <div style="padding: 1.5rem 1.5rem 0 1.5rem;">
                <div id="active-banner" class="status-banner">
                    <div class="status-info">
                        <div class="dot-glow"></div>
                        <div>
                            <div class="status-label">Volume Mounted Successfully</div>
                            <div id="active-banner-details" class="status-subtext">Mounted /dev/disk4 at ./mnt</div>
                        </div>
                    </div>
                </div>
            </div>

            <div id="console-body" class="console-body">
                <!-- Log entries will be appended here -->
            </div>
        </div>
    </div>

    <script>
        const consoleBody = document.getElementById('console-body');
        let streamSource = null;

        function addLogEntry(time, source, text) {
            const entry = document.createElement('div');
            entry.className = 'log-entry';
            
            const timeSpan = document.createElement('span');
            timeSpan.className = 'log-time';
            timeSpan.textContent = '[' + time + ']';
            
            const sourceSpan = document.createElement('span');
            sourceSpan.className = 'log-source source-' + source;
            sourceSpan.textContent = source;
            
            const textSpan = document.createElement('span');
            textSpan.className = 'log-text';
            textSpan.textContent = text;
            
            entry.appendChild(timeSpan);
            entry.appendChild(sourceSpan);
            entry.appendChild(textSpan);
            
            consoleBody.appendChild(entry);
            consoleBody.scrollTop = consoleBody.scrollHeight;
        }

        function clearConsole() {
            consoleBody.innerHTML = '';
        }

        async function loadDisks() {
            try {
                const res = await fetch('/api/disks');
                const disks = await res.json();
                const select = document.getElementById('format-device');
                select.innerHTML = '<option value="">-- Choose target --</option>';
                disks.forEach(d => {
                    const opt = document.createElement('option');
                    opt.value = d.device;
                    opt.textContent = d.device + ' (' + d.size + ' - ' + d.type + ')';
                    select.appendChild(opt);
                });
            } catch (err) {
                addLogEntry(new Date().toLocaleTimeString(), 'system', 'Error fetching disks: ' + err.message);
            }
        }

        async function updateStatus() {
            try {
                const res = await fetch('/api/status');
                const status = await res.json();
                
                document.getElementById('platform-info').textContent = 'OS: ' + status.platform;
                document.getElementById('arch-info').textContent = 'Arch: ' + status.architecture;

                const mountBtn = document.getElementById('mount-btn');
                const unmountBtn = document.getElementById('unmount-btn');
                const statusGlow = document.getElementById('status-glow');
                const banner = document.getElementById('active-banner');
                const bannerDetails = document.getElementById('active-banner-details');

                if (status.active) {
                    mountBtn.style.display = 'none';
                    unmountBtn.style.display = 'block';
                    statusGlow.className = 'dot-glow';
                    banner.style.display = 'flex';
                    bannerDetails.textContent = 'Mounted ' + status.device + ' at ' + status.mountpoint;
                    
                    // Sync inputs
                    document.getElementById('mount-device').value = status.device;
                    document.getElementById('mount-point').value = status.mountpoint;
                } else {
                    mountBtn.style.display = 'block';
                    unmountBtn.style.display = 'none';
                    statusGlow.className = 'dot-glow inactive';
                    banner.style.display = 'none';
                }
            } catch (err) {
                console.error('Failed to get status', err);
            }
        }

        async function formatDevice() {
            const devSelect = document.getElementById('format-device').value;
            const devCustom = document.getElementById('format-custom').value.trim();
            const device = devCustom || devSelect;
            const label = document.getElementById('format-label').value.trim();

            if (!device) {
                alert('Please select or specify a device path');
                return;
            }

            try {
                const res = await fetch('/api/format', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ device, label })
                });
                
                if (!res.ok) {
                    const text = await res.text();
                    addLogEntry(new Date().toLocaleTimeString(), 'mkfs', 'Error starting format: ' + text);
                } else {
                    // Pre-populate mount device
                    document.getElementById('mount-device').value = device;
                }
            } catch (err) {
                addLogEntry(new Date().toLocaleTimeString(), 'mkfs', 'Error: ' + err.message);
            }
        }

        async function mountDevice() {
            const device = document.getElementById('mount-device').value.trim();
            const mountpoint = document.getElementById('mount-point').value.trim();

            if (!device || !mountpoint) {
                alert('Device and Mount Point are required');
                return;
            }

            try {
                const res = await fetch('/api/mount', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ device, mountpoint })
                });

                if (!res.ok) {
                    const text = await res.text();
                    addLogEntry(new Date().toLocaleTimeString(), 'mount', 'Error starting mount: ' + text);
                } else {
                    setTimeout(updateStatus, 1000);
                }
            } catch (err) {
                addLogEntry(new Date().toLocaleTimeString(), 'mount', 'Error: ' + err.message);
            }
        }

        async function unmountDevice() {
            try {
                const res = await fetch('/api/unmount', { method: 'POST' });
                if (!res.ok) {
                    const text = await res.text();
                    addLogEntry(new Date().toLocaleTimeString(), 'mount', 'Error unmounting: ' + text);
                } else {
                    setTimeout(updateStatus, 1000);
                }
            } catch (err) {
                addLogEntry(new Date().toLocaleTimeString(), 'mount', 'Error: ' + err.message);
            }
        }

        function initLogStream() {
            // Load history first
            fetch('/api/logs/history')
                .then(res => res.json())
                .then(history => {
                    history.forEach(log => {
                        addLogEntry(log.timestamp, log.Source, log.Text);
                    });
                })
                .then(() => {
                    // Connect stream
                    if (streamSource) streamSource.close();
                    streamSource = new EventSource('/api/logs/stream');
                    streamSource.onmessage = (event) => {
                        const log = JSON.parse(event.data);
                        addLogEntry(log.timestamp, log.Source, log.Text);
                    };
                    streamSource.onerror = () => {
                        console.log('SSE connection lost, reconnecting...');
                    };
                });
        }

        // Initialize
        loadDisks();
        updateStatus();
        initLogStream();
        
        // Status polling loop
        setInterval(updateStatus, 3000);
    </script>
</body>
</html>
`
