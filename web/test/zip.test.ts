import assert from 'node:assert/strict';
import { test } from 'node:test';
import { deflateRawSync } from 'node:zlib';
import { unzip, zip } from '../app/utils/zip.ts';

const text = (data: Uint8Array) => new TextDecoder().decode(data);
const bytes = (s: string) => new TextEncoder().encode(s);

test('a zip gives back its files, names in UTF-8', async () => {
  const files = await unzip(
    zip([
      { name: 'Réunion.md', data: bytes('## Réunion #travail') },
      { name: 'Vide.md', data: bytes('') },
    ]),
  );
  assert.deepEqual(
    files.map((f) => [f.name, text(f.data)]),
    [
      ['Réunion.md', '## Réunion #travail'],
      ['Vide.md', ''],
    ],
  );
});

test('a deflated entry of another archiver is inflated, a folder left out', async () => {
  const content = '# Jour\n\n'.repeat(50);
  const archive = zip([
    { name: 'dossier/', data: new Uint8Array() },
    { name: 'dossier/Jour.md', data: deflateRawSync(bytes(content)) },
  ]);
  // Marked deflated, as other archivers write it: the method sits at 8 in the local header and 10 in the central one.
  const view = new DataView(archive.buffer);
  const central = view.getUint32(archive.length - 22 + 16, true);
  const second = central + 46 + 'dossier/'.length;
  view.setUint16(second + 10, 8, true);
  const files = await unzip(archive);
  assert.deepEqual(
    files.map((f) => [f.name, text(f.data)]),
    [['dossier/Jour.md', content]],
  );
});

test('a file that is no zip is refused', async () => {
  await assert.rejects(unzip(bytes('# not a zip')));
});
