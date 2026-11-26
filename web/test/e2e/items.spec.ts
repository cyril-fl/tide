import { expect, test } from '@playwright/test';
import { sandbox } from './helpers';

let box: Awaited<ReturnType<typeof sandbox>>;
test.beforeEach(async () => (box = await sandbox()));
test.afterEach(async () => box.dispose());

test('a note dragged onto a folder moves into it', async ({ page }) => {
  await box.api.post('/api/folders', { data: { title: 'Destination', parent_id: box.folder.id } });
  await box.note('# Voyageuse');
  await page.goto(`/folders/${box.folder.id}`);
  const main = page.getByRole('main');

  await main.getByRole('link', { name: /Voyageuse/ }).dragTo(main.getByRole('link', { name: /Destination/ }));
  await expect(main.getByRole('link', { name: /Voyageuse/ })).toBeHidden();
  await main.getByRole('link', { name: /Destination/ }).click();
  await expect(main.getByRole('link', { name: /Voyageuse/ })).toBeVisible();
});

test('a note moved from its right-click menu goes into the folder picked, ⌘Z brings it back', async ({ page }) => {
  await box.api.post('/api/folders', { data: { title: 'Destination', parent_id: box.folder.id } });
  await box.note('# Voyageuse');
  await page.goto(`/folders/${box.folder.id}`);
  const main = page.getByRole('main');

  await main.getByRole('link', { name: /Voyageuse/ }).click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Déplacer vers…' }).click();
  await page.getByPlaceholder('Déplacer vers…').fill('Destination');
  await page.getByRole('option', { name: `/${box.prefix}/Destination` }).click();
  await expect(main.getByRole('link', { name: /Voyageuse/ })).toBeHidden();
  await expect(page.getByRole('dialog')).toBeHidden();

  await page.keyboard.press('ControlOrMeta+z');
  await expect(main.getByRole('link', { name: /Voyageuse/ })).toBeVisible();
});

test('a removed note comes back from the trash, then leaves it for good', async ({ page }) => {
  const note = await box.note(`# Recyclée ${box.prefix}`);
  await page.goto(`/folders/${box.folder.id}`);
  const main = page.getByRole('main');

  await main.getByRole('link', { name: /Recyclée/ }).click({ button: 'right' });
  // The open menu hides the page from the roles: wait for the removal itself, or the navigation cancels it.
  const removed = page.waitForResponse((response) => response.url().endsWith(`/api/notes/${note.id}`));
  await page.getByRole('menuitem', { name: 'Supprimer' }).click();
  await removed;
  await expect(main.getByRole('link', { name: /Recyclée/ })).toBeHidden();

  await page.goto('/trash');
  const row = main.getByRole('listitem').filter({ hasText: `Recyclée ${box.prefix}` });
  await row.getByRole('button', { name: 'Restaurer' }).click();
  await expect(row).toBeHidden();
  await page.goto(`/folders/${box.folder.id}`);
  await expect(main.getByRole('link', { name: /Recyclée/ })).toBeVisible();

  await box.api.delete(`/api/notes/${note.id}`);
  await page.goto('/trash');
  await row.getByRole('button', { name: 'Supprimer définitivement' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Supprimer' }).click();
  await expect(row).toBeHidden();
  expect((await box.api.post('/api/restore', { data: { ids: [note.id] } })).ok()).toBe(true);
  expect(await (await box.api.get(`/api/notes/${note.id}`)).status()).toBe(404); // Nothing left to restore.
});

// The folder goes to the trash while its new note is open (another device): the note is refused in French, not lost with it (#63).
test('a new note in a trashed folder is refused with a French text', async ({ page }) => {
  await page.goto(`/notes/new?parent=${box.folder.id}`);
  await box.api.delete(`/api/folders/${box.folder.id}`);

  await page.locator('.ProseMirror').click();
  await page.keyboard.type('Perdue ?');
  await expect(page.getByText('Le dossier parent est dans la corbeille').first()).toBeVisible();
  const items = await (await box.api.get(`/api/items?parent=${box.folder.id}`)).json();
  expect(items).toHaveLength(0);
});

// 201 notes: one page more than the web app reads at once (#142).
const pages = Array.from({ length: 201 }, (_, i) => `# Page ${String(i).padStart(3, '0')}`);

// A big folder reads its first page only, then the next one when scrolled to its end (#142).
test('a folder bigger than a page reads its notes as it scrolls', async ({ page }) => {
  await box.notes(pages);
  const reads: string[] = [];
  page.on(
    'request',
    (request) => request.url().includes(`/api/items?parent=${box.folder.id}`) && reads.push(request.url()),
  );
  await page.goto(`/folders/${box.folder.id}`);
  const main = page.getByRole('main');

  await expect(main.getByRole('link', { name: /Page 000/ })).toBeVisible();
  expect(reads).toEqual([expect.stringContaining('limit=200&offset=0')]);
  await main.hover();
  await page.mouse.wheel(0, 100_000);
  await expect(main.getByRole('link', { name: /Page 200/ })).toBeVisible();
  expect(reads).toHaveLength(2);
});

// A tag and a search with a page more than the web app reads at once render only the rows in sight (#143).
for (const [name, path, api] of [
  ['the page of a tag', (tag: string) => `/tags/${tag}`, (tag: string) => `/api/tags/${tag}?`],
  ['a search', (tag: string) => `/search?q=${tag}`, (tag: string) => `/api/search?q=${tag}&`],
] as const)
  test(`${name} bigger than a page reads its notes as it scrolls`, async ({ page }) => {
    const tag = box.prefix;
    const notes = pages.map((content) => `${content}\n\n#${tag}`);
    // A search ranks the latest edited first (#170): Page 200 is made first and Page 000 last, alone.
    await box.note(notes[200]!);
    await box.notes(notes.slice(1, 200));
    await box.note(notes[0]!);
    try {
      const reads: string[] = [];
      page.on('request', (request) => request.url().includes(api(tag)) && reads.push(request.url()));
      await page.goto(path(tag));
      const main = page.getByRole('main');

      await expect(main.getByRole('link', { name: /Page 000/ })).toBeVisible();
      expect(reads).toEqual([expect.stringContaining('limit=200&offset=0')]);
      expect(await main.getByRole('link').count()).toBeLessThan(100);
      await main.hover();
      await page.mouse.wheel(0, 100_000);
      await expect(main.getByRole('link', { name: /Page 200/ })).toBeVisible();
      expect(reads).toHaveLength(2);
    } finally {
      await box.api.delete(`/api/tags/${tag}`);
    }
  });

// The tree learns from has_folders that a folder opens, and reads only folders, never the notes of a big one (#140).
test('the sidebar tree opens a folder without reading its notes', async ({ page }) => {
  const branch = await (
    await box.api.post('/api/folders', { data: { title: 'Branche', parent_id: box.folder.id } })
  ).json();
  await box.api.post('/api/folders', { data: { title: 'Feuille', parent_id: branch.id } });
  await box.note('# Cachée');
  const reads: string[] = [];
  page.on('request', (request) => request.url().includes('/api/items') && reads.push(request.url()));

  await page.goto('/');
  const tree = page.getByRole('navigation');
  const row = tree.getByRole('listitem').filter({ has: page.getByRole('link', { name: box.prefix, exact: true }) });
  await row.getByRole('button', { name: 'Déplier' }).first().click();
  const branchRow = row.getByRole('listitem').filter({ hasText: 'Branche' });
  await branchRow.getByRole('button', { name: 'Déplier' }).click();
  await expect(branchRow.getByRole('link', { name: 'Feuille' })).toBeVisible();
  await expect(branchRow.getByRole('listitem').filter({ hasText: 'Feuille' }).getByRole('button')).toHaveCount(0);
  await expect(row.getByText('Cachée')).toHaveCount(0); // The recent notes above may show it, the tree not.
  expect(reads.filter((url) => !url.includes('kind=folder') && url.includes(box.folder.id))).toEqual([]);
});

// Every row asks for the account (through usePro): the app reads it once, not once per row (#156).
test('opening a folder reads the account once', async ({ page }) => {
  await box.notes(Array.from({ length: 40 }, (_, i) => `# Ligne ${i}`));
  const reads = { me: 0, folders: 0, recent: 0 };
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname.replace('/api/', '');
    if (path in reads) reads[path as keyof typeof reads]++;
  });
  await page.goto(`/folders/${box.folder.id}`);

  await expect(page.getByRole('main').getByRole('link', { name: /Ligne/ }).first()).toBeVisible();
  await page.waitForLoadState('networkidle');
  expect(reads).toEqual({ me: 1, folders: 1, recent: 1 });
});

// Neutral by name, then choices added in turn, kept on the account for the other devices.
test('a folder sorts by several choices, back to neutral', async ({ page }) => {
  const older = await box.note('# Ancienne');
  await box.note('# Brouillon');
  const pinned = await box.note('# Zénith');
  await box.api.patch(`/api/notes/${pinned.id}`, { data: { pinned: true } });
  await box.api.patch(`/api/notes/${older.id}`, { data: { content: '# Ancienne\n\nretouchée' } });
  await box.api.patch('/api/me', { data: { sort: '' } });
  await page.goto(`/folders/${box.folder.id}`);
  // Not by role: the open menu hides the page from the roles. ul: the breadcrumb's crumbs are li of an ol.
  const titles = page.locator('main ul > li');
  const saved = () => expect.poll(async () => (await (await box.api.get('/api/me')).json()).sort);

  try {
    await expect(titles).toHaveText([/Ancienne/, /Brouillon/, /Zénith/]);
    // A choice goes ascending, then descending, then off; the menu stays open between clicks.
    await page.getByRole('button', { name: 'Trier par', exact: true }).click();
    await page.getByRole('menuitem', { name: 'Épinglés' }).click();
    await expect(titles).toHaveText([/Zénith/, /Ancienne/, /Brouillon/]);
    const modified = page.getByRole('menuitem', { name: 'Date de modification' });
    await modified.click();
    await expect(titles).toHaveText([/Zénith/, /Brouillon/, /Ancienne/]); // pinned, then the oldest edit
    await modified.click();
    await expect(titles).toHaveText([/Zénith/, /Ancienne/, /Brouillon/]);
    await saved().toBe('pinned,-updated');
    await page.getByRole('menuitem', { name: 'Épinglés en premier' }).click();
    await page.getByRole('menuitem', { name: 'Épinglés en dernier' }).click();
    await saved().toBe('-updated');
    await page.getByRole('menuitem', { name: 'Réinitialiser le tri' }).click();
    await expect(titles).toHaveText([/Ancienne/, /Brouillon/, /Zénith/]);
    await saved().toBe('');
  } finally {
    await box.api.patch('/api/me', { data: { sort: '' } });
  }
});

// The list's headers sort like the menu, and the name's keeps folders or notes only.
test('the list headers sort and filter by kind', async ({ page }) => {
  await box.api.post('/api/folders', { data: { title: 'Classeur', parent_id: box.folder.id } });
  await box.note('# Alpha');
  await box.note('# Bêta');
  await box.api.patch('/api/me', { data: { sort: '' } });
  await page.goto(`/folders/${box.folder.id}`);
  const titles = page.locator('main ul > li');

  try {
    await expect(titles).toHaveText([/Classeur/, /Alpha/, /Bêta/]);
    const byName = page.getByRole('button', { name: 'Trier par Nom' });
    await byName.click();
    await byName.click(); // ascending, then descending; folders stay first
    await expect(titles).toHaveText([/Classeur/, /Bêta/, /Alpha/]);
    await expect.poll(async () => (await (await box.api.get('/api/me')).json()).sort).toBe('-title');

    await page.getByRole('button', { name: 'Nom', exact: true }).click();
    await page.getByRole('menuitemcheckbox', { name: 'Notes' }).click();
    await expect(titles).toHaveText([/Bêta/, /Alpha/]);
    await page.getByRole('button', { name: 'Notes', exact: true }).click();
    await page.getByRole('menuitemcheckbox', { name: 'Dossiers' }).click();
    await expect(titles).toHaveText([/Classeur/]);
  } finally {
    await box.api.patch('/api/me', { data: { sort: '' } });
  }
});
