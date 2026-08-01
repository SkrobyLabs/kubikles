import { rm } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from 'vite';
import { validateBuildVersion, verifyArtifact } from './accelerator-browser-artifact.mjs';

const frontendRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const output = path.join(frontendRoot, 'dist/accelerator-browser');
const marker = path.join(output, '.kubikles-browser-v1.json');

await rm(marker, { force: true });
const version = validateBuildVersion(process.env.BUILD_VERSION);
try {
  await build({ configFile: path.join(frontendRoot, 'vite.accelerator-browser.config.ts') });
  await verifyArtifact(output, version);
} catch (error) {
  await rm(marker, { force: true });
  throw error;
}
