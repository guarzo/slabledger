const puppeteer = require('puppeteer');

let browserPromise = null;

async function getBrowser() {
  if (!browserPromise) {
    browserPromise = puppeteer.launch({
      headless: true,
      args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
        '--disable-gpu',
      ],
    }).then(b => {
      b.on('disconnected', () => {
        browserPromise = null;
      });
      return b;
    });
  }
  return browserPromise;
}

/**
 * Renders a single slide and returns JPEG bytes.
 *
 * @param {string} baseURL  - e.g. "http://localhost:3001"
 * @param {string} postId   - UUID of the social post
 * @param {number} slideIndex - 0 = cover, N = card at index N-1
 * @param {number} timeoutMs - per-slide timeout (default 30000ms)
 * @returns {Promise<Buffer>} JPEG bytes
 */
async function renderSlide(baseURL, postId, slideIndex, timeoutMs = 30000) {
  const b = await getBrowser();
  const page = await b.newPage();
  try {
    await page.setViewport({ width: 1080, height: 1080, deviceScaleFactor: 2 });

    const url = `${baseURL}/slide-preview?postId=${encodeURIComponent(postId)}&slideIndex=${slideIndex}`;
    await page.goto(url, { waitUntil: 'networkidle2', timeout: timeoutMs });

    // Wait for the [data-slide] element to appear
    await page.waitForSelector('[data-slide]', { timeout: timeoutMs });

    // Wait for all images within [data-slide] to finish loading
    await page.evaluate(() => {
      const imgs = document.querySelectorAll('[data-slide] img');
      return Promise.all(Array.from(imgs).map(img => {
        if (img.complete) return Promise.resolve();
        return new Promise((resolve) => {
          img.addEventListener('load', resolve, { once: true });
          img.addEventListener('error', resolve, { once: true });
        });
      }));
    });

    // Small settle delay to allow CSS animations to render
    await new Promise(r => setTimeout(r, 200));

    const element = await page.$('[data-slide]');
    if (!element) throw new Error('No [data-slide] element found on page');

    const jpeg = await element.screenshot({
      type: 'jpeg',
      quality: 90,
    });
    return jpeg;
  } finally {
    try { await page.close(); } catch (_) {}
  }
}

/**
 * Renders all slides for a post (cover + one per card).
 *
 * @param {string} baseURL
 * @param {string} postId
 * @param {number} cardCount
 * @returns {Promise<Buffer[]>} Array of JPEG buffers, index 0 = cover
 */
async function renderPost(baseURL, postId, cardCount) {
  const results = [];
  for (let i = 0; i <= cardCount; i++) {
    const jpeg = await renderSlide(baseURL, postId, i);
    results.push(jpeg);
  }
  return results;
}

module.exports = { renderPost };
