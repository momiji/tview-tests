// Package ui renders the live traffic table in the terminal with tview, when
// the proxy is started with --ui. It reads the shared traffic table and
// refreshes a few times per second until the context is cancelled or the user
// quits (q / Ctrl-C).
package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	ratecounter "github.com/enterprizesoftware/rate-counter"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"test/internal/ui/traffic"
)

const (
	rowActive = iota
	rowStalled
	rowRemoved
	rowHeader
)

type stateRow struct {
	row   *traffic.TrafficRow
	state int
	order int
}

type renderer struct {
	screen tcell.Screen
	app    *tview.Application
	table  *tview.Table
	data   *traffic.TrafficTable
}

// RunTraffic displays the traffic table until ctx is cancelled or the user
// quits. It blocks; the caller should run the proxy server in the background.
func RunTraffic(ctx context.Context, data *traffic.TrafficTable) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	r := &renderer{screen: screen, data: data}
	r.table = tview.NewTable().
		ScrollToBeginning().
		SetBorders(false).
		SetFixed(1, 0).
		SetSelectable(false, false).
		SetSeparator(tview.Borders.Vertical)
	r.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey { return nil })

	r.app = tview.NewApplication().SetScreen(screen)
	r.app.SetRoot(r.table, true)
	r.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyCtrlC:
			r.app.Stop()
		case event.Key() == tcell.KeyRune && (event.Rune() == 'q' || event.Rune() == 'Q'):
			r.app.Stop()
		}
		return nil
	})

	// stop the app when the context is cancelled (proxy shutdown)
	go func() {
		<-ctx.Done()
		r.app.Stop()
	}()
	// refresh loop
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				r.app.QueueUpdateDraw(r.update)
			}
		}
	}()

	err = r.app.Run()
	close(done)
	return err
}

func (r *renderer) update() {
	screenWidth, screenHeight := r.screen.Size()
	totalWidth := 5 + 15 + 15 + 7 + 7 + 15 + 2
	urlWidth := screenWidth - totalWidth
	if urlWidth < 20 {
		urlWidth = 20
	}
	r.setRow(0, rowHeader, urlWidth, "ID", "URL", "RECV", "SENT", "RECV/S", "SENT/S")
	trafficRows := r.data.RowsCopy()
	rowsToDisplay := len(trafficRows)
	if rowsToDisplay+1 >= screenHeight {
		rowsToDisplay = screenHeight - 1
	}
	stateRows := make([]*stateRow, rowsToDisplay)
	i := 0
	for _, row := range slices.Backward(trafficRows) {
		if i >= rowsToDisplay {
			break
		}
		state := rowActive
		order := 0
		if row.Removed.IsZero() {
			updated := row.LastSend
			if row.LastReceive.After(updated) {
				updated = row.LastReceive
			}
			if time.Since(updated) > 1*time.Second {
				state = rowStalled
			}
		} else {
			state = rowRemoved
			order = 1
		}
		stateRows[i] = &stateRow{row: row, state: state, order: order}
		i++
	}
	slices.SortStableFunc(stateRows, func(r1, r2 *stateRow) int {
		switch {
		case r1.order < r2.order:
			return -1
		case r1.order > r2.order:
			return 1
		}
		return 0
	})
	for i, sr := range stateRows {
		row := sr.row
		r.setRow(i+1, sr.state, urlWidth, strconv.Itoa(int(row.ReqId)), row.Url,
			bytesFormat(row.BytesSentPerSecond), bytesFormat(row.BytesReceivedPerSecond),
			rateFormat(row.BytesSentPerSecond), rateFormat(row.BytesReceivedPerSecond))
	}
	// remove any extra rows
	for i := r.table.GetRowCount() - 1; i > rowsToDisplay; i-- {
		r.table.RemoveRow(i)
	}
	// remove hidden rows
	for i := screenHeight; i < r.table.GetRowCount(); i++ {
		r.table.RemoveRow(i)
	}
}

func (r *renderer) setRow(row int, state int, urlWidth int, reqId, url, bytesSent, bytesReceived, bytesSentPerSecond, bytesReceivedPerSecond string) {
	r.setCell(row, 0, reqId, 5, false, state)
	r.setCell(row, 1, url, -urlWidth, true, state)
	r.setCell(row, 2, bytesReceived, 15, false, state)
	r.setCell(row, 3, bytesSent, 15, false, state)
	r.setCell(row, 4, bytesReceivedPerSecond, 7, false, state)
	r.setCell(row, 5, bytesSentPerSecond, 7, false, state)
}

func (r *renderer) setCell(i, j int, s string, w int, left bool, state int) {
	align := tview.AlignRight
	if left {
		align = tview.AlignLeft
	}
	length := tview.TaggedStringWidth(s)
	if w > 0 {
		if length < w {
			if left {
				s += strings.Repeat(" ", w-length)
			} else {
				s = strings.Repeat(" ", w-length) + s
			}
		}
	} else if w < 0 {
		if length > -w {
			s = s[:-w-1] + "…"
		} else if length < -w {
			if left {
				s += strings.Repeat(" ", -w-length)
			} else {
				s = strings.Repeat(" ", -w-length) + s
			}
		}
	}
	if i > 0 && j == 1 {
		a := strings.Split(s, " ")
		if len(a) > 1 {
			a[1] = "[aqua]" + a[1] + "[-]"
		}
		if len(a) > 3 {
			a[3] = "[yellow]" + a[3] + "[-]"
		}
		s = strings.Join(a, " ")
	}
	s = " " + s + " "
	// style
	color := tcell.ColorWhite
	bgcolor := tcell.ColorBlack
	switch state {
	case rowActive:
		color = tcell.ColorGreen
	case rowStalled:
		color = tcell.ColorOrange
	case rowRemoved:
		color = tcell.ColorGrey
	case rowHeader:
		color = tcell.ColorBlack
		bgcolor = tcell.ColorAqua
	}
	r.table.SetCell(i, j, r.table.GetCell(i, j).SetAlign(align).SetTextColor(color).SetBackgroundColor(bgcolor).SetText(s))
}

func bytesFormat(rate *ratecounter.Rate) string {
	return humanize.Comma(int64(rate.Total()))
}

func rateFormat(rate *ratecounter.Rate) string {
	return strings.ReplaceAll(humanize.IBytes(uint64(rate.RatePer(1*time.Second))), "i", "")
}
