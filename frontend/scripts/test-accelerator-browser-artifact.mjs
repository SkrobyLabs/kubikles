import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { build } from 'vite';
import { ARTIFACT_FILES, verifyArtifact } from './accelerator-browser-artifact.mjs';

const frontendRoot = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const version = process.env.BUILD_VERSION;
const configFile = path.join(frontendRoot, 'vite.accelerator-browser.config.ts');
const temporary = await mkdtemp(path.join(os.tmpdir(), 'kubikles-accelerator-browser-'));
const acceptanceRoot = process.env.ACCELERATOR_ACCEPTANCE_ARTIFACT_ROOT;
if (acceptanceRoot && !path.isAbsolute(acceptanceRoot)) throw new Error('Accelerator acceptance artifact root must be absolute');

async function currentArtifactRoot() {
  if (!acceptanceRoot) return path.join(frontendRoot, 'dist/accelerator-browser');
  const source = path.join(acceptanceRoot, 'browser-artifact');
  const output = path.join(temporary, 'staged-browser-artifact');
  await mkdir(path.join(output, 'assets'), { recursive: true });
  for (const file of ARTIFACT_FILES) await cp(path.join(source, file), path.join(output, file));
  return output;
}

async function buildInto(name, buildVersion) {
  const output = path.join(temporary, name);
  process.env.BUILD_VERSION = buildVersion;
  await build({ configFile, build: { outDir: output, emptyOutDir: true } });
  return output;
}

try {
  const current = await verifyArtifact(await currentArtifactRoot(), version);
  const firstRoot = await buildInto('first', version);
  const secondRoot = await buildInto('second', version);
  const first = await verifyArtifact(firstRoot, version);
  const second = await verifyArtifact(secondRoot, version);
  if (JSON.stringify(first) !== JSON.stringify(second)) throw new Error('Accelerator Browser builds are not deterministic');
  for (const clean of [first, second]) {
    if (JSON.stringify(current.hashes) !== JSON.stringify(clean.hashes) || current.aggregate !== clean.aggregate) {
      throw new Error('Current Accelerator Browser output differs from clean build');
    }
  }
  const changedVersion = `${version}-changed`;
  const changedRoot = await buildInto('changed', changedVersion);
  const changed = await verifyArtifact(changedRoot, changedVersion);
  for (const file of ['assets/browser.js', 'assets/browser.css']) {
    if (changed.hashes[file] !== first.hashes[file]) throw new Error(`${file} changed when only BUILD_VERSION changed`);
  }
  if (changed.hashes['.kubikles-browser-v1.json'] === first.hashes['.kubikles-browser-v1.json'] || changed.aggregate === first.aggregate) {
    throw new Error('BUILD_VERSION did not change artifact identity');
  }
  const tampered = path.join(temporary, 'tampered');
  await cp(firstRoot, tampered, { recursive: true });
  await writeFile(path.join(tampered, '.kubikles-browser-v1.json'), `${await readFile(path.join(tampered, '.kubikles-browser-v1.json'), 'utf8')} `);
  let rejected = false;
  try { await verifyArtifact(tampered, version); } catch { rejected = true; }
  if (!rejected) throw new Error('Tampered marker was accepted');
} finally {
  process.env.BUILD_VERSION = version;
  await rm(temporary, { recursive: true, force: true });
}
