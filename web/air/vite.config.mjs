import { defineConfig, loadEnv, transformWithOxc } from 'vite';
import react from '@vitejs/plugin-react';
import svgr from 'vite-plugin-svgr';
import { fileURLToPath } from 'node:url';
import { legacyConfig } from '../legacy/vite-config.mjs';

const root = fileURLToPath(new URL('.', import.meta.url));
export default defineConfig(({ mode }) => legacyConfig({
  root: root.replace(/[\\/]$/, ''), mode, react, svgr, transformWithOxc,
  publicEnv: loadEnv(mode, root, 'REACT_APP_'),
}));
