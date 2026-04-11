const express = require('express');
const path = require('path');
const FormData = require('form-data');
const { renderPost } = require('./renderer');

const app = express();
const PORT = parseInt(process.env.PORT || '3001', 10);
const STATIC_DIR = path.join(__dirname, 'dist');

// The base URL used by Puppeteer to navigate to SlidePreviewPage.
// Within the container, this is always localhost:PORT.
const INTERNAL_BASE_URL = `http://localhost:${PORT}`;

app.use(express.json({ limit: '10mb' }));

// Serve compiled frontend build
app.use(express.static(STATIC_DIR));

// Health check
app.get('/health', (_req, res) => {
  res.json({ status: 'ok' });
});

// Render all slides for a post
// Body: SocialPostDetail JSON (must include cardCount or cards.length)
app.post('/render/:postId', async (req, res) => {
  const { postId } = req.params;
  const body = req.body;

  // Derive card count from body
  const cardCount = body.cardCount ?? (Array.isArray(body.cards) ? body.cards.length : 0);

  try {
    const jpegs = await renderPost(INTERNAL_BASE_URL, postId, cardCount);

    const form = new FormData();
    jpegs.forEach((buf, i) => {
      form.append(`slide-${i}`, buf, {
        filename: `slide-${i}.jpg`,
        contentType: 'image/jpeg',
      });
    });

    res.set('Content-Type', `multipart/form-data; boundary=${form.getBoundary()}`);
    form.on('error', (err) => {
      console.error(`Form stream error for post ${postId}:`, err);
      res.destroy(err);
    });
    form.pipe(res);
  } catch (err) {
    console.error(`Render failed for post ${postId}:`, err);
    res.status(500).json({ error: err.message || 'render failed' });
  }
});

// SPA fallback: return index.html for any non-API, non-static path
app.get('*', (_req, res) => {
  res.sendFile(path.join(STATIC_DIR, 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Render service listening on :${PORT}`);
});
