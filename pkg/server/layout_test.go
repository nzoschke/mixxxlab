package server

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayout(t *testing.T) {
	// Change to project root directory
	err := os.Chdir("../..")
	require.NoError(t, err, "Failed to change to project root")

	// Start server on test port
	e := echo.New()
	e.HideBanner = true
	e.GET("/", serveIndex)
	e.Static("/src", "src")
	e.GET("/api/music", listMusic)
	e.GET("/api/music/*", serveMusic)

	go func() {
		e.Start(":18080")
	}()
	defer e.Close()

	// Wait for server to start
	for i := 0; i < 50; i++ {
		resp, err := http.Get("http://localhost:18080/")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Launch browser
	l := launcher.New().Headless(true)
	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	// Add timestamp to bust cache
	page := browser.MustPage(fmt.Sprintf("http://localhost:18080/?t=%d", time.Now().UnixNano()))
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

	// Wait for LitElement to fully render
	page.MustEval(`() => {
		return new Promise(resolve => {
			const check = () => {
				const app = document.querySelector('mixx-app');
				if (app && app.shadowRoot && app.shadowRoot.querySelector('header')) {
					resolve();
				} else {
					setTimeout(check, 50);
				}
			};
			check();
		});
	}`)

	// Get viewport height
	viewportHeight := page.MustEval(`() => window.innerHeight`).Int()
	t.Logf("Viewport height: %d", viewportHeight)

	// Get all layout info at once via JS (handles Shadow DOM)
	layout := page.MustEval(`() => {
		const app = document.querySelector('mixx-app');
		if (!app || !app.shadowRoot) {
			return { error: 'App or shadow root not found' };
		}

		const header = app.shadowRoot.querySelector('header');
		const sidebar = app.shadowRoot.querySelector('.sidebar');
		const main = app.shadowRoot.querySelector('.main');

		const result = {
			viewportHeight: window.innerHeight,
			htmlOverflow: window.getComputedStyle(document.documentElement).overflow,
			bodyOverflow: window.getComputedStyle(document.body).overflow,
		};

		if (header) {
			const rect = header.getBoundingClientRect();
			result.header = {
				top: rect.top,
				height: rect.height,
				bottom: rect.bottom
			};
		}

		if (sidebar) {
			const rect = sidebar.getBoundingClientRect();
			const style = window.getComputedStyle(sidebar);
			result.sidebar = {
				top: rect.top,
				height: rect.height,
				bottom: rect.bottom,
				overflowY: style.overflowY,
				scrollHeight: sidebar.scrollHeight,
				clientHeight: sidebar.clientHeight
			};
		}

		if (main) {
			const rect = main.getBoundingClientRect();
			result.main = {
				top: rect.top,
				height: rect.height,
				bottom: rect.bottom
			};
		}

		return result;
	}`).Map()

	// Check for errors
	if errVal, ok := layout["error"]; ok && errVal.Str() != "" {
		t.Fatalf("Layout error: %s", errVal.Str())
	}

	t.Logf("Layout: %+v", layout)

	// Check if header exists
	if _, ok := layout["header"]; !ok {
		t.Fatal("Header not found in layout")
	}

	// Test 1: Header exists and is at top
	headerData := layout["header"].Map()
	require.NotNil(t, headerData, "Header should exist")
	headerTop := headerData["top"].Num()
	headerHeight := headerData["height"].Num()
	t.Logf("Header: top=%f, height=%f", headerTop, headerHeight)
	assert.InDelta(t, 0, headerTop, 2, "Header should be at top of page")
	assert.Greater(t, headerHeight, float64(30), "Header should have height")

	// Test 2: Sidebar exists and fills height below header
	sidebarData := layout["sidebar"].Map()
	require.NotNil(t, sidebarData, "Sidebar should exist")
	sidebarTop := sidebarData["top"].Num()
	sidebarHeight := sidebarData["height"].Num()
	sidebarBottom := sidebarData["bottom"].Num()
	t.Logf("Sidebar: top=%f, height=%f, bottom=%f", sidebarTop, sidebarHeight, sidebarBottom)
	assert.InDelta(t, headerHeight, sidebarTop, 2, "Sidebar should start below header")
	expectedSidebarHeight := float64(viewportHeight) - headerHeight
	assert.InDelta(t, expectedSidebarHeight, sidebarHeight, 10,
		"Sidebar should fill remaining height (expected %f, got %f)", expectedSidebarHeight, sidebarHeight)

	// Test 3: Main area exists and fills height below header
	mainData := layout["main"].Map()
	require.NotNil(t, mainData, "Main should exist")
	mainTop := mainData["top"].Num()
	mainHeight := mainData["height"].Num()
	t.Logf("Main: top=%f, height=%f", mainTop, mainHeight)
	assert.InDelta(t, headerHeight, mainTop, 2, "Main should start below header")
	assert.Greater(t, mainHeight, float64(100), "Main should have substantial height")

	// Test 4: Sidebar should have overflow-y: auto or scroll
	overflowY := sidebarData["overflowY"].Str()
	t.Logf("Sidebar overflow-y: %s", overflowY)
	assert.True(t, overflowY == "auto" || overflowY == "scroll",
		"Sidebar should have overflow-y: auto or scroll, got: %s", overflowY)

	// Test 5: Page itself should not scroll
	htmlOverflow := layout["htmlOverflow"].Str()
	bodyOverflow := layout["bodyOverflow"].Str()
	t.Logf("Page overflow - html: %s, body: %s", htmlOverflow, bodyOverflow)
	assert.True(t, htmlOverflow == "hidden" || bodyOverflow == "hidden",
		"Page should not scroll (html: %s, body: %s)", htmlOverflow, bodyOverflow)

	// Test 6: Verify sidebar can scroll when content overflows
	scrollHeight := sidebarData["scrollHeight"].Num()
	clientHeight := sidebarData["clientHeight"].Num()
	t.Logf("Sidebar scroll: scrollHeight=%f, clientHeight=%f", scrollHeight, clientHeight)

	if scrollHeight > clientHeight {
		canScroll := page.MustEval(`() => {
			const sidebar = document.querySelector('mixx-app').shadowRoot.querySelector('.sidebar');
			const initialScroll = sidebar.scrollTop;
			sidebar.scrollTop = 100;
			const newScroll = sidebar.scrollTop;
			sidebar.scrollTop = initialScroll;
			return newScroll > initialScroll;
		}`).Bool()
		require.True(t, canScroll, "Sidebar should be scrollable when content overflows")
		t.Log("Sidebar scrolls correctly!")
	} else {
		t.Log("Sidebar content fits without scrolling")
	}

	fmt.Println("All layout tests passed!")
}
