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
 * @param {object|null} postData - SocialPostDetail payload to serve locally (avoids backend fetch)
 * @param {number} timeoutMs - per-slide timeout (default 30000ms)
 * @returns {Promise<Buffer>} JPEG bytes
 */
async function renderSlide(baseURL, postId, slideIndex, postData = null, timeoutMs = 30000) {
  const b = await getBrowser();
  const page = await b.newPage();
  try {
    await page.setViewport({ width: 1080, height: 1080, deviceScaleFactor: 2 });

    // Intercept the API call for this post and respond with the provided data,
    // avoiding a second round-trip to the backend.
    if (postData) {
      await page.setRequestInterception(true);
      page.on('request', (interceptedReq) => {
        const url = interceptedReq.url();
        if (url.includes(`/api/social/posts/${encodeURIComponent(postId)}`) ||
            url.includes(`/api/social/posts/${postId}`)) {
          interceptedReq.respond({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify(postData),
          });
        } else {
          interceptedReq.continue();
        }
      });
    }

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
 * @param {object|null} postData - SocialPostDetail payload to serve locally
 * @returns {Promise<Buffer[]>} Array of JPEG buffers, index 0 = cover
 */
async function renderPost(baseURL, postId, cardCount, postData = null) {
  const promises = [];
  for (let i = 0; i <= cardCount; i++) {
    promises.push(renderSlide(baseURL, postId, i, postData));
  }
  return Promise.all(promises);
}

module.exports = { renderPost };
