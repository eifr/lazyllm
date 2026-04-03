#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const https = require('https');
const { spawnSync } = require('child_process');
const os = require('os');

const packageJson = require('../package.json');
const VERSION = packageJson.version;
const REPO = 'eifr/lazyllm';

const PLATFORM_MAPPING = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows',
};

const ARCH_MAPPING = {
  x64: 'amd64',
  arm64: 'arm64',
};

const platform = PLATFORM_MAPPING[process.platform];
const arch = ARCH_MAPPING[process.arch];

if (!platform || !arch) {
  console.error(`Unsupported platform/architecture: ${process.platform}-${process.arch}`);
  process.exit(1);
}

const ext = platform === 'windows' ? '.exe' : '';
const binName = `lazyllm_${platform}_${arch}${ext}`;
const tag = `v${VERSION}`; 
const url = `https://github.com/${REPO}/releases/download/${tag}/${binName}`;

const cacheDir = path.join(os.homedir(), '.lazyllm-cache');
if (!fs.existsSync(cacheDir)) {
  fs.mkdirSync(cacheDir, { recursive: true });
}

const cachedBinPath = path.join(cacheDir, `${tag}-${binName}`);

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    
    function fetch(currentUrl) {
      https.get(currentUrl, (response) => {
        if (response.statusCode === 301 || response.statusCode === 302) {
          fetch(response.headers.location);
        } else if (response.statusCode !== 200) {
          fs.unlink(dest, () => reject(new Error(`Failed to download: ${response.statusCode} - ${response.statusMessage}`)));
        } else {
          response.pipe(file);
          file.on('finish', () => {
            file.close(resolve);
          });
        }
      }).on('error', (err) => {
        fs.unlink(dest, () => reject(err));
      });
    }
    
    fetch(url);
  });
}

async function main() {
  if (!fs.existsSync(cachedBinPath)) {
    console.error(`Downloading lazyllm ${tag} for ${platform}-${arch}...`);
    try {
      await download(url, cachedBinPath);
      if (platform !== 'windows') {
        fs.chmodSync(cachedBinPath, 0o755);
      }
    } catch (err) {
      console.error(`Error downloading binary from ${url}:`, err.message);
      console.error('Please ensure the version is published as a GitHub Release.');
      process.exit(1);
    }
  }

  const args = process.argv.slice(2);
  const result = spawnSync(cachedBinPath, args, { stdio: 'inherit' });
  
  if (result.error) {
    console.error('Error running lazyllm:', result.error.message);
    process.exit(1);
  }
  
  process.exit(result.status !== null ? result.status : 1);
}

main();
