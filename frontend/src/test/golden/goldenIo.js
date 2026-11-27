// File access for the golden-parity test. Kept in plain JS so the TypeScript build (which has no
// Node typings) never has to type-check Node APIs, like the other file-reading tests here.
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
export const GOLDEN_DIR = resolve(here, '../../../../pkg/rows/testdata/golden');

export const regenerate = () => process.env.GOLDEN === '1';
export const goldenPath = type => resolve(GOLDEN_DIR, `${type}.json`);

export function writeGolden(type, text) {
    mkdirSync(GOLDEN_DIR, { recursive: true });
    writeFileSync(goldenPath(type), text);
}

export function readGolden(type) {
    const file = goldenPath(type);
    return existsSync(file) ? readFileSync(file, 'utf8') : null;
}
