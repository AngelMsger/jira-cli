'use strict';

const path = require('path');
const test = require('node:test');
const assert = require('node:assert/strict');

const { assetName, binPath, welcomeText } = require('./install.js');

test('selects Windows release assets for both supported architectures', () => {
  assert.equal(assetName('win32', 'x64'), 'jira-cli-windows-amd64.exe');
  assert.equal(assetName('win32', 'arm64'), 'jira-cli-windows-arm64.exe');
});

test('uses an exe launcher target on Windows', () => {
  assert.equal(path.basename(binPath('win32').file), 'jira-cli.exe');
});

test('rejects unsupported Windows architectures', () => {
  assert.throws(() => assetName('win32', 'ia32'), /unsupported platform win32\/ia32/);
});

test('welcome text recommends valid Jira commands', () => {
  assert.match(welcomeText(), /jira-cli issue search/);
  assert.match(welcomeText(), /jira-cli issue get/);
  assert.doesNotMatch(welcomeText(), /jira-cli (search|page)/);
});
