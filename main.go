package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

const defaultURL = "http://localhost:7777/variables.html"

// loadDotEnv читает KEY=VALUE построчно из .env и выставляет
// переменные окружения, не перезаписывая уже заданные извне.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

var (
	rePosition    = regexp.MustCompile(`<p id="position">(-?\d+)</p>`)
	reDuration    = regexp.MustCompile(`<p id="duration">(-?\d+)</p>`)
	rePositionStr = regexp.MustCompile(`<p id="positionstring">([^<]*)</p>`)
	reDurationStr = regexp.MustCompile(`<p id="durationstring">([^<]*)</p>`)
	reStateString = regexp.MustCompile(`<p id="statestring">([^<]*)</p>`)
	reFile        = regexp.MustCompile(`<p id="file">([^<]*)</p>`)
)

type MPCState struct {
	Position    int64
	Duration    int64
	PositionStr string
	DurationStr string
	StateString string
	File        string
}

func extractInt(html string, re *regexp.Regexp) int64 {
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.ParseInt(m[1], 10, 64)
	return v
}

func extractStr(html string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func fetchState(url string, client *http.Client) (*MPCState, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)

	return &MPCState{
		Position:    extractInt(html, rePosition),
		Duration:    extractInt(html, reDuration),
		PositionStr: extractStr(html, rePositionStr),
		DurationStr: extractStr(html, reDurationStr),
		StateString: extractStr(html, reStateString),
		File:        extractStr(html, reFile),
	}, nil
}

type winRect struct {
	Left, Top, Right, Bottom int32
}

type winPoint struct {
	X, Y int32
}

type winMonitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

// screenLayout возвращает рабочую область (без панели задач) и полные границы
// монитора, на котором сейчас находится курсор мыши — а не всегда основного монитора.
func screenLayout() (work, monitor winRect) {
	user32 := syscall.NewLazyDLL("user32.dll")
	getCursorPos := user32.NewProc("GetCursorPos")
	monitorFromPoint := user32.NewProc("MonitorFromPoint")
	getMonitorInfoW := user32.NewProc("GetMonitorInfoW")

	const monitorDefaultToNearest = 2

	var pt winPoint
	getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	ptPacked := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	hMonitor, _, _ := monitorFromPoint.Call(ptPacked, monitorDefaultToNearest)

	var mi winMonitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	getMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))

	return mi.RcWork, mi.RcMonitor
}

func formatTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func main() {
	loadDotEnv(".env")

	envURL := os.Getenv("MPC_URL")
	if envURL == "" {
		envURL = defaultURL
	}

	urlFlag := flag.String("url", envURL,
		"URL страницы variables.html MPC-HC (или переменная окружения MPC_URL)")
	intervalFlag := flag.Duration("interval", 500*time.Millisecond,
		"Интервал опроса")
	flag.Parse()

	a := app.New()
	w := a.NewWindow("MPC-HC Progress")

	windowWidth, windowHeight := float32(900), float32(170)
	posX, posY := 0, 0
	if work, monitor := screenLayout(); work.Right > work.Left {
		windowWidth = float32(work.Right - work.Left)
		windowHeight = float32(monitor.Bottom-monitor.Top) * 0.15
		posX, posY = int(work.Left), int(work.Top)
	}

	w.Resize(fyne.NewSize(windowWidth, windowHeight))
	if dw, ok := w.(desktop.Window); ok {
		// Fyne игнорирует RequestPosition(0, 0), считая это "позиция не задана",
		// поэтому сдвигаем на 1px, когда рабочая область начинается в углу экрана.
		if posX == 0 && posY == 0 {
			posX = 1
		}
		dw.RequestPosition(posX, posY)
	}

	// --- Виджеты ---
	fileLabel := widget.NewLabel("Ожидание подключения к MPC-HC...")
	fileLabel.Wrapping = fyne.TextWrapWord
	fileLabel.Alignment = fyne.TextAlignCenter

	elapsedLabel := widget.NewLabelWithStyle("00:00",
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true})
	remainingLabel := widget.NewLabelWithStyle("00:00",
		fyne.TextAlignTrailing, fyne.TextStyle{Bold: true, Monospace: true})

	progressBar := widget.NewProgressBar() // по умолчанию 0..1

	statusLabel := widget.NewLabelWithStyle("",
		fyne.TextAlignCenter, fyne.TextStyle{})

	progressRow := container.NewBorder(nil, nil, elapsedLabel, remainingLabel, progressBar)

	content := container.NewVBox(
		fileLabel,
		statusLabel,
		progressRow,
	)
	w.SetContent(container.NewPadded(content))

	client := &http.Client{Timeout: 2 * time.Second}

	// --- Цикл опроса в отдельной горутине ---
	go func() {
		ticker := time.NewTicker(*intervalFlag)
		defer ticker.Stop()

		for range ticker.C {
			st, err := fetchState(*urlFlag, client)
			if err != nil {
				// fyne.Do — безопасное обновление UI из горутины (Fyne 2.6+)
				fyne.Do(func() {
					statusLabel.SetText("⚠ Нет связи с MPC-HC: " + err.Error())
				})
				continue
			}

			// Копируем, чтобы захватить в замыкание
			stCopy := st

			fyne.Do(func() {
				if stCopy.Duration <= 0 {
					statusLabel.SetText("Нет активного воспроизведения")
					progressBar.SetValue(0)
					elapsedLabel.SetText("00:00")
					remainingLabel.SetText("00:00")
					return
				}

				progress := float64(stCopy.Position) / float64(stCopy.Duration)
				if progress < 0 {
					progress = 0
				}
				if progress > 1 {
					progress = 1
				}
				progressBar.SetValue(progress)

				elapsed := stCopy.PositionStr
				if elapsed == "" {
					elapsed = formatTime(stCopy.Position)
				}
				remaining := formatTime(stCopy.Duration - stCopy.Position)

				elapsedLabel.SetText(elapsed)
				remainingLabel.SetText("-" + remaining)
				statusLabel.SetText(fmt.Sprintf("%.1f%%   %s   (из %s)",
					progress*100, stCopy.StateString, formatTime(stCopy.Duration)))
				fileLabel.SetText(stCopy.File)
			})
		}
	}()

	w.ShowAndRun()
}
