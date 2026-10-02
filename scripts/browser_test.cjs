const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const { spawn } = require('node:child_process');
const { chromium } = require('playwright');

async function freePort() {
  const server = net.createServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise(resolve => server.close(resolve));
  return port;
}

(async () => {
  const dir = process.argv[2];
  const port = await freePort();
  const origin = `http://127.0.0.1:${port}`;
  const server = spawn(path.join(dir, 'site'), [], { cwd: dir, env: {
    ...process.env, ADDR: `127.0.0.1:${port}`, SITE_URL: origin, SITE_NAME: 'Runtime Site',
    SITE_AUTHOR: 'Runtime Author', SITE_DESCRIPTION: 'Runtime Description',
    ADMIN_PASSWORD: 'browser-test-password', ADMIN_SESSION_SECRET: 'browser-test-signing-key-32-characters-long',
    DATABASE_PATH: path.join(dir, 'browser.db'), ANALYTICS_DATABASE_PATH: path.join(dir, 'custom-analytics.db'),
  }, stdio: ['ignore', 'pipe', 'pipe'] });
  let logs = '';
  for (const stream of [server.stdout, server.stderr]) stream.on('data', data => { logs = (logs + data).slice(-65536); });
  let browser, page;
  const errors = [];
  try {
    let ready = false;
    for (let i = 0; i < 100; i++) {
      if (server.exitCode !== null) throw new Error('Generated server exited: ' + logs);
      try { if ((await fetch(origin)).ok) { ready = true; break; } } catch (_) {}
      await new Promise(resolve => setTimeout(resolve, 50));
    }
    assert(ready, 'server did not start');
    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ permissions: ['clipboard-read', 'clipboard-write'] });
    await context.addInitScript(() => {
      window.__analyticsEvents = [];
      const sendBeacon = navigator.sendBeacon.bind(navigator);
      navigator.sendBeacon = (url, data) => {
        if (String(url).includes('/api/analytics/collect')) data.text().then(body => window.__analyticsEvents.push(JSON.parse(body)));
        return sendBeacon(url, data);
      };
    });
    page = await context.newPage();
    page.on('pageerror', error => errors.push(error.message));
    await page.goto(origin);
    assert((await page.title()).includes('Runtime Site'));
    const css = await page.request.get(origin + '/public/tailwind.css');
    assert.equal(css.status(), 200); assert((await css.body()).length > 1000);
    assert(fs.existsSync(path.join(dir, 'custom-analytics.db')));
    await page.goto(origin + '/admin/');
    await page.locator('[name=password]').fill('browser-test-password');
    await page.getByRole('button', { name: 'Log In', exact: true }).click();
    await page.getByRole('button', { name: 'New Post', exact: true }).click();
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await page.waitForURL('**/admin/save/');
    assert((await page.title()).includes('Edit post'));
    assert(await page.getByRole('alert').isVisible());
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.waitForURL('**/admin/');
    // Browser requests use Fetch Metadata in Echo 4.16; explicitly exercise the
    // cookie/token fallback for the non-browser API requests used to seed data.
    await page.request.get(origin + '/admin/');
    const csrf = (await context.cookies()).find(c => c.name === '_csrf').value;
    for (let i = 0; i < 23; i++) {
      const slug = 'post-' + String(i).padStart(3, '0');
      const response = await page.request.post(origin + '/admin/save/', { form: {
        _csrf: csrf, slug, title: slug, date: '2026-01-01', tags: 'go', summary: 'Summary ' + slug, content: '**Body** ' + slug + '\n\n## A clearer reading experience\n\nArticles need comfortable spacing and readable text on every screen.\n\n- Read on desktop\n- Keep reading on mobile\n\n```go\n' + 'long code sample '.repeat(25) + '\n```', published: '1',
      } });
      assert.equal(response.status(), 200);
    }
    await page.goto(origin);
    assert.equal(await page.locator('main article').count(), 20);
    assert.equal(await page.locator('main h1').textContent(), 'Stories & ideas');
    if (process.env.PUBENGINE_SCREENSHOT_DIR) {
      fs.mkdirSync(process.env.PUBENGINE_SCREENSHOT_DIR, { recursive: true });
      await page.screenshot({ path: path.join(process.env.PUBENGINE_SCREENSHOT_DIR, 'home-desktop.png'), fullPage: true });
    }
    await page.getByRole('link', { name: 'Next', exact: true }).click();
    assert.equal(await page.locator('main article').count(), 3);
    await page.goto(origin);
    await page.locator('a[href="/blog/post-000/"]').click();
    await page.waitForURL('**/blog/post-000/');
    assert.equal(await page.locator('main h1').textContent(), 'post-000');
    assert.equal(await page.locator('link[rel=canonical]').getAttribute('href'), origin + '/blog/post-000/');
    assert((await page.locator('script[type="application/ld+json"]').textContent()).includes('Runtime Author'));
    assert.equal(await page.locator('aside a').count(), 6);
    await page.waitForFunction(() => window.__analyticsEvents.some(e => e.event === 'view' && e.path === '/blog/post-000/'));
    assert.equal(await page.evaluate(() => window.__analyticsEvents.filter(e => e.event === 'view' && e.path === '/blog/post-000/').length), 1);
    assert.equal(await page.evaluate(() => document.activeElement.id), 'content');
    assert(await page.locator('.prose').evaluate(el => parseFloat(getComputedStyle(el.querySelector('h2')).fontSize) > parseFloat(getComputedStyle(el.querySelector('p')).fontSize)), 'article headings need typography styles');
    await page.setViewportSize({ width: 390, height: 844 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'article overflows mobile viewport');
    if (process.env.PUBENGINE_SCREENSHOT_DIR) await page.screenshot({ path: path.join(process.env.PUBENGINE_SCREENSHOT_DIR, 'article-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goBack();
    await page.locator('main article').first().waitFor();
    assert.equal(new URL(page.url()).pathname, '/');
    assert.equal(await page.locator('meta[name=description]').getAttribute('content'), 'Runtime Description');
    await page.route('**/blog/post-001/**', route => route.fulfill({ status: 503, body: 'Unavailable' }));
    const failure = page.waitForResponse(response => response.status() === 503);
    await page.locator('a[href="/blog/post-001/"]').click();
    await failure;
    assert.equal(new URL(page.url()).pathname, '/');
    assert.equal(await page.locator('main article').count(), 20);
    await page.getByRole('alert').filter({ hasText: 'could not be loaded' }).waitFor();
    await page.unroute('**/blog/post-001/**');
    let release, started;
    const hold = new Promise(resolve => { release = resolve; });
    const pending = new Promise(resolve => { started = resolve; });
    await page.route('**/blog/post-000/**', async route => { started(); await hold; await route.continue().catch(() => {}); });
    await page.locator('a[href="/blog/post-000/"]').click(); await pending;
    await page.locator('a[href="/blog/post-002/"]').click(); await page.waitForURL('**/blog/post-002/');
    release(); await page.unroute('**/blog/post-000/**');
    assert.equal(await page.locator('main h1').textContent(), 'post-002');
    await page.goto(origin + '/admin/');
    await page.getByRole('button', { name: 'Edit', exact: true }).first().click();
    await page.setViewportSize({ width: 390, height: 844 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'editor overflows mobile viewport');
    if (process.env.PUBENGINE_SCREENSHOT_DIR) await page.screenshot({ path: path.join(process.env.PUBENGINE_SCREENSHOT_DIR, 'editor-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.locator('[name=content]').fill('Unsaved draft');
    const slug = await page.locator('[name=slug]').inputValue();
    const revision = await page.locator('[name=revision]').inputValue();
    await page.request.post(origin + '/admin/save/', { form: { _csrf: csrf, slug, original_slug: slug, revision, title: 'Changed elsewhere', date: '2026-01-01', content: 'Other draft', published: '1' } });
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await page.waitForURL('**/admin/save/');
    assert((await page.getByRole('alert').textContent()).includes('reload'));
    assert.equal(await page.locator('[name=content]').inputValue(), 'Unsaved draft');
    await page.goto(origin + '/admin/');
    await page.getByRole('button', { name: 'Images', exact: true }).click();
    const png = await page.evaluate(() => { const c = document.createElement('canvas'); c.width = 12; c.height = 8; return c.toDataURL('image/png').split(',')[1]; });
    await page.locator('[name=image]').setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: Buffer.from(png, 'base64') });
    const uploadedResponse = page.waitForResponse(r => r.url().includes('/admin/images/upload/'));
    await page.getByRole('button', { name: 'Upload', exact: true }).click();
    const uploaded = await uploadedResponse;
    assert.equal(uploaded.status(), 200, await uploaded.text());
    await page.getByRole('button', { name: 'Copy Markdown', exact: true }).click();
    const markdown = await page.evaluate(() => navigator.clipboard.readText());
    assert(markdown.endsWith('{|12|8}'));
    await page.route('**/admin/images/upload/', route => route.fulfill({ status: 500, body: 'Failed' }));
    await page.locator('[name=image]').setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: Buffer.from(png, 'base64') });
    await page.getByRole('button', { name: 'Upload', exact: true }).click();
    await page.locator('#post-form [role=alert]').waitFor();
    assert(await page.locator('[name=image]').isVisible());
    await page.unroute('**/admin/images/upload/');
    await page.goto(origin + '/admin/analytics/');
    await page.getByRole('button', { name: 'Setup', exact: true }).click();
    await page.getByText('Quick Setup', { exact: true }).waitFor();
    const snippet = await page.locator('.code-block').textContent();
    assert(snippet.includes('/public/analytics.js?v=2'));
    assert.equal((await page.request.get(origin + '/public/analytics.js?v=2')).status(), 200);
    await page.goto(origin + '/admin/');
    await context.clearCookies();
    await page.getByRole('button', { name: 'Images', exact: true }).click();
    await page.locator('[name=password]').waitFor();
    assert.deepEqual(errors, []);
    console.log('Generated-site browser checks passed');
  } catch (error) {
    console.error(logs, errors);
    if (page && await page.locator('#post-form').count()) console.error(await page.locator('#post-form').innerText());
    throw error;
  } finally {
    if (browser) await browser.close();
    server.kill('SIGTERM');
    const kill = setTimeout(() => server.kill('SIGKILL'), 5000); kill.unref();
    await new Promise(resolve => { if (server.exitCode !== null) resolve(); else server.once('exit', resolve); });
    clearTimeout(kill);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
