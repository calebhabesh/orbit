import { spawn, execFileSync } from 'child_process';
import fs from 'fs';
import path from 'path';
import puppeteer from '../web/node_modules/puppeteer-core/lib/puppeteer/puppeteer-core.js';

async function main() {
  const rootDir = process.cwd();
  const binary = path.join(rootDir, 'bin', 'orbit');
  const tempDir = fs.mkdtempSync('/tmp/orbit-p14-');
  const stateDir = path.join(tempDir, 'state');
  const workspaceDir = path.join(tempDir, 'workspace');
  const screenshotsDir = path.join(rootDir, 'docs', 'evidence', 'p14-20260923', 'screenshots');

  fs.mkdirSync(stateDir, { mode: 0o700, recursive: true });
  fs.chmodSync(stateDir, 0o700);
  fs.mkdirSync(workspaceDir, { recursive: true });
  fs.mkdirSync(screenshotsDir, { recursive: true });

  console.log(`Setting up test environment in ${tempDir}...`);

  // 1. Initialize
  execFileSync(binary, ['init', '--state', stateDir], { stdio: 'inherit' });

  // 2. Register folder
  const folderID = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
  execFileSync(binary, ['engine', 'register', '--state', stateDir, '--folder', folderID, '--root', workspaceDir], { stdio: 'inherit' });

  // 3. Populate files in workspace
  const notesFile = path.join(workspaceDir, 'notes.txt');
  fs.writeFileSync(notesFile, 'Initial notes version 1\n');

  const scriptFile = path.join(workspaceDir, 'deploy.sh');
  fs.writeFileSync(scriptFile, '#!/bin/sh\necho "Deploying Orbit..."\n', { mode: 0o755 });

  const archFile = path.join(workspaceDir, 'architecture.md');
  fs.writeFileSync(archFile, '# Architecture Overview\n\nCausal synchronization with local SQLite persistence.\n');

  // Initial scan
  execFileSync(binary, ['engine', 'scan', '--state', stateDir, '--folder', folderID], { stdio: 'inherit' });

  // Update notes.txt to version 2
  fs.writeFileSync(notesFile, 'Updated notes version 2 with additional insights and tasks.\n');
  execFileSync(binary, ['engine', 'scan', '--state', stateDir, '--folder', folderID], { stdio: 'inherit' });

  // 3b. Inject concurrent conflict for architecture.md
  console.log('Injecting concurrent conflict for architecture.md...');
  execFileSync('go', ['run', 'scripts/create_conflict.go', stateDir, folderID], { stdio: 'inherit' });

  // 4. Start serve in background
  console.log('Starting orbit serve with control listener...');
  const server = spawn(binary, ['serve', '--state', stateDir, '--control-listen', '127.0.0.1:0', '--no-watch'], {
    stdio: ['ignore', 'pipe', 'inherit'],
  });

  let controlURL = '';
  await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Timeout waiting for control listener')), 5000);
    server.stdout.on('data', (chunk) => {
      const text = chunk.toString();
      const match = text.match(/control-listener=(http:\/\/[^\s]+)/);
      if (match) {
        controlURL = match[1];
        clearTimeout(timeout);
        resolve(controlURL);
      }
    });
  });

  console.log(`Control listener is active at: ${controlURL}`);

  // 5. Generate bootstrap token
  const bootOut = execFileSync(binary, ['engine', 'control', 'bootstrap-token', '--state', stateDir, '--json'], { encoding: 'utf-8' });
  const bootData = JSON.parse(bootOut);
  const bootstrapToken = bootData.bootstrap_token;
  console.log(`Generated bootstrap token: ${bootstrapToken}`);

  // 6. Launch Chromium headless via puppeteer-core
  console.log('Launching Chromium to capture UI screenshots...');
  const browser = await puppeteer.launch({
    executablePath: '/usr/bin/chromium',
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-gpu'],
  });

  try {
    const page = await browser.newPage();
    page.on('console', (msg) => console.log('BROWSER LOG:', msg.text()));
    page.on('pageerror', (err) => console.log('BROWSER ERROR:', err));
    page.on('response', (res) => {
      if (res.status() >= 400) {
        console.log(`HTTP ${res.status()} ${res.url()}`);
      }
    });
    await page.setViewport({ width: 1280, height: 800 });

    // --- SCREENSHOT 1: Bootstrap Login Screen ---
    console.log('Capturing Screenshot 1: Bootstrap Login Screen...');
    await page.goto(controlURL + '/', { waitUntil: 'networkidle0' });
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-01-bootstrap-login.png') });

    // Submit bootstrap token
    console.log('Authenticating with bootstrap token...');
    await page.type('#bootstrap-token', bootstrapToken);
    await page.click('button[type="submit"]');

    // Wait for Dashboard
    await page.waitForSelector('header.app-header', { timeout: 5000 });
    await new Promise((r) => setTimeout(r, 600)); // allow state to settle

    // --- SCREENSHOT 2: Folders, Devices & Work Dashboard ---
    console.log('Capturing Screenshot 2: Folders & Work Dashboard...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-02-folders-work.png') });

    // --- SCREENSHOT 3: Doctor Health Inspection Modal ---
    console.log('Opening Doctor Health modal...');
    const doctorBtn = await page.waitForSelector('button[title*="Doctor Health"]');
    await doctorBtn.click();
    await page.waitForSelector('.modal-content', { timeout: 3000 });
    await new Promise((r) => setTimeout(r, 400));
    console.log('Capturing Screenshot 3: Doctor Health Modal...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-03-doctor-modal.png') });

    // Close Doctor Modal
    const closeBtn = await page.$('.modal-footer button');
    if (closeBtn) await closeBtn.click();
    await new Promise((r) => setTimeout(r, 300));

    // --- SCREENSHOT 4: Files & History View ---
    console.log('Navigating to Files & History view...');
    const navButtons = await page.$$('.nav-tab');
    for (const btn of navButtons) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.includes('Files & History')) {
        await btn.click();
        break;
      }
    }
    await page.waitForSelector('table.data-table', { timeout: 3000 });

    // Click "History" button on notes.txt
    const historyBtns = await page.$$('button');
    for (const btn of historyBtns) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.trim() === 'History') {
        await btn.click();
        break;
      }
    }
    await new Promise((r) => setTimeout(r, 600));
    console.log('Capturing Screenshot 4: Files & History View...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-04-files-history.png') });

    // --- SCREENSHOT 5: Historical Restore Preview Modal ---
    console.log('Opening Restore Preview modal...');
    const restoreBtns = await page.$$('button');
    for (const btn of restoreBtns) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.trim() === 'Restore') {
        await btn.click();
        break;
      }
    }
    await page.waitForSelector('#restore-title', { timeout: 3000 });
    await new Promise((r) => setTimeout(r, 400));
    console.log('Capturing Screenshot 5: Restore Preview Modal...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-05-restore-preview.png') });

    // Close restore modal
    const cancelRestoreBtn = await page.$('.modal-footer button');
    if (cancelRestoreBtn) await cancelRestoreBtn.click();
    await new Promise((r) => setTimeout(r, 300));

    // --- SCREENSHOT 6: Conflicts View ---
    console.log('Navigating to Conflicts view...');
    for (const btn of navButtons) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.includes('Conflicts')) {
        await btn.click();
        break;
      }
    }
    await new Promise((r) => setTimeout(r, 500));
    console.log('Capturing Screenshot 6: Conflicts View...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-06-conflicts-view.png') });

    // --- SCREENSHOT 7: Conflict Resolve Modal ---
    console.log('Opening Conflict Resolution modal...');
    const resolveBtns = await page.$$('button');
    for (const btn of resolveBtns) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.includes('Resolve Conflict')) {
        await btn.click();
        break;
      }
    }
    await page.waitForSelector('#conflict-modal-title', { timeout: 3000 });
    await new Promise((r) => setTimeout(r, 400));
    console.log('Capturing Screenshot 7: Conflict Resolve Modal...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-07-conflict-resolve-modal.png') });

    // Close conflict modal
    const closeConflictBtn = await page.$('.modal-footer button');
    if (closeConflictBtn) await closeConflictBtn.click();
    await new Promise((r) => setTimeout(r, 300));

    // --- SCREENSHOT 8: Mobile Responsive Viewport (390px) ---
    console.log('Setting mobile viewport (390x844)...');
    await page.setViewport({ width: 390, height: 844 });
    // Switch back to Folders tab
    const freshNav = await page.$$('.nav-tab');
    for (const btn of freshNav) {
      const text = await (await btn.getProperty('textContent')).jsonValue();
      if (text.includes('Folders & Work')) {
        await btn.click();
        break;
      }
    }
    await new Promise((r) => setTimeout(r, 500));
    console.log('Capturing Screenshot 8: Mobile Responsive View...');
    await page.screenshot({ path: path.join(screenshotsDir, 'screenshot-08-mobile-responsive.png') });

    console.log('All screenshots captured successfully!');
  } finally {
    await browser.close();
    server.kill('SIGTERM');
    try {
      fs.rmSync(tempDir, { recursive: true, force: true });
    } catch {}
  }
}

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
