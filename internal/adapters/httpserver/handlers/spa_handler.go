package handlers

import (
	"fmt"
	"net/http"
	"os"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

type SPAHandler struct {
	logger observability.Logger
}

func NewSPAHandler(logger observability.Logger) *SPAHandler {
	return &SPAHandler{logger: logger}
}

func (h *SPAHandler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	h.serveFrontend(w, r)
}

func (h *SPAHandler) serveFrontend(w http.ResponseWriter, r *http.Request) {
	indexPath := "web/dist/index.html"

	// Prevent browser from caching HTML responses so hashed asset URLs stay current after rebuilds
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	//nolint:errcheck // response already committed; write error unactionable
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Frontend Not Built</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            max-width: 800px;
            margin: 80px auto;
            padding: 0 20px;
            line-height: 1.6;
            color: #333;
        }
        h1 {
            color: #e74c3c;
            border-bottom: 3px solid #e74c3c;
            padding-bottom: 10px;
        }
        .code-block {
            background: #f4f4f4;
            border-left: 4px solid #3498db;
            padding: 15px;
            margin: 20px 0;
            font-family: 'Courier New', monospace;
            overflow-x: auto;
        }
        .step {
            margin: 20px 0;
        }
        .step-number {
            display: inline-block;
            background: #3498db;
            color: white;
            border-radius: 50%%;
            width: 30px;
            height: 30px;
            text-align: center;
            line-height: 30px;
            margin-right: 10px;
            font-weight: bold;
        }
        .note {
            background: #fff3cd;
            border-left: 4px solid #ffc107;
            padding: 15px;
            margin: 20px 0;
        }
        a {
            color: #3498db;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <h1>Frontend Assets Not Found</h1>

    <p>The web frontend has not been built yet. The server is running correctly, but the static assets (HTML, CSS, JavaScript) are missing.</p>

    <div class="step">
        <span class="step-number">1</span>
        <strong>Navigate to the web directory:</strong>
        <div class="code-block">cd web</div>
    </div>

    <div class="step">
        <span class="step-number">2</span>
        <strong>Install dependencies (first time only):</strong>
        <div class="code-block">npm install</div>
    </div>

    <div class="step">
        <span class="step-number">3</span>
        <strong>Build the frontend:</strong>
        <div class="code-block">npm run build</div>
    </div>

    <div class="step">
        <span class="step-number">4</span>
        <strong>Restart the server</strong>
        <p>Return to the project root and restart the server:</p>
        <div class="code-block">cd ..<br>go run ./cmd/slabledger server</div>
    </div>

    <div class="note">
        <strong>Note:</strong> For development with live reload, you can run <code>npm run dev</code> in the web directory instead of building. The development server typically runs on port 5173.
    </div>

    <div class="note">
        <strong>API Endpoints:</strong> The REST API is still available. You can test endpoints at:
        <ul>
            <li><a href="/api/health">/api/health</a> - Health check</li>
            <li><a href="/api/sets">/api/sets</a> - List available sets</li>
        </ul>
    </div>
</body>
</html>`)
}
