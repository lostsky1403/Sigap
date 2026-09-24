#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const target = process.argv[2];
if (!target) {
  console.error('Usage: node scripts/apply-design-tokens.mjs <target-css-path>');
  process.exit(1);
}

const here = path.dirname(fileURLToPath(import.meta.url));
const source = path.resolve(here, '..', 'variables.css');
const destination = path.resolve(process.cwd(), target);
fs.mkdirSync(path.dirname(destination), { recursive: true });
fs.copyFileSync(source, destination);
console.log(`Copied design tokens to ${destination}`);
