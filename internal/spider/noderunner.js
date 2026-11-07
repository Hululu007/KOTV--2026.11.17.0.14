'use strict';

/**
 * 猫爪 Node 源包启动器（仅用 Node 内置模块）。
 * 用法: node noderunner.js <index.js> [index.config.js]
 * 环境: HOST=127.0.0.1 DEV_HTTP_PORT=<port> NODE_PATH=<dataDir> KOTV_MSG_PORT=<enginePort>
 */
const http = require('http');
const path = require('path');
const fs = require('fs');

const msgPort = parseInt(process.env.KOTV_MSG_PORT || '0', 10) || 0;

globalThis.catDartServerPort = function catDartServerPort() {
  return msgPort;
};

globalThis.catServerFactory = function catServerFactory(handler) {
  const server = http.createServer(handler);
  server.on('listening', () => {
    const addr = server.address();
    if (addr && typeof addr === 'object' && addr.port) {
      process.stdout.write('NODE_PORT=' + addr.port + '\n');
    }
  });
  return server;
};

const indexPath = process.argv[2];
if (!indexPath) {
  console.error('usage: noderunner.js <index.js> [index.config.js]');
  process.exit(1);
}

let config = {};
const configPath = process.argv[3];
if (configPath && fs.existsSync(configPath)) {
  try {
    // eslint-disable-next-line import/no-dynamic-require, global-require
    let loaded = require(path.resolve(configPath));
    if (loaded && typeof loaded === 'object' && loaded.default) {
      loaded = loaded.default;
    }
    if (loaded && typeof loaded === 'object') {
      config = loaded;
    }
  } catch (err) {
    console.error('load index.config.js failed:', err && err.message ? err.message : err);
  }
}

// eslint-disable-next-line import/no-dynamic-require, global-require
const mod = require(path.resolve(indexPath));
const start = (mod && mod.start) || (mod && mod.default && mod.default.start);
if (typeof start !== 'function') {
  console.error('index.js does not export start()');
  process.exit(1);
}

Promise.resolve(start(config)).catch((err) => {
  console.error('start failed:', err && err.stack ? err.stack : err);
  process.exit(1);
});
