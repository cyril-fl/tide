import { expect, test } from '@playwright/test';
import { cursorAtEnd, sandbox } from './helpers';

let box: Awaited<ReturnType<typeof sandbox>>;
test.beforeEach(async () => (box = await sandbox()));
test.afterEach(async () => box.dispose());

// The Ultra tabs of the editor (#128): a note opened replaces the tab of the one shown, a tab of its own is asked for
// from the menu of a note, one note open shows no bar; a note no longer found loses its tab, a tab closed opens its
// neighbour, the others or all of them close from the bar, a tab dragged moves, they survive a reload and they go
// away with the Ultra plan.
test('the Ultra tabs keep the open notes', async ({ page }) => {
  const second = await box.note(`# Second ${box.prefix}`);
  const third = await box.note(`# Third ${box.prefix}`);
  const fourth = await box.note(`# Fourth ${box.prefix}`);
  const first = await box.note(
    `# First ${box.prefix}\n\n[Second](note:${second.id}) [Third](note:${third.id}) [Fourth](note:${fourth.id})`,
  );
  const bar = page.getByRole('navigation', { name: 'Notes ouvertes' });
  const titles = () => bar.getByRole('link').allTextContents();
  const inTab = async (name: string) => {
    await page.locator(`[data-note-id="${first.id}"] a[data-wikilink]`, { hasText: name }).click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
  };
  try {
    await box.api.patch('/api/me', { data: { plan: 'ultra' } });

    // One note open: no bar. Another opened from the address replaces it.
    await page.goto(`/notes/${second.id}`);
    await expect(page.getByRole('textbox').getByRole('heading', { name: `Second ${box.prefix}` })).toBeVisible();
    await page.goto(`/notes/${first.id}`);
    await expect(page.getByRole('textbox').getByRole('heading', { name: `First ${box.prefix}` })).toBeVisible();
    await expect(bar).toHaveCount(0);

    // A tab of its own opens after the one shown.
    await inTab('Second');
    await expect(page).toHaveURL(`/notes/${second.id}`);
    await expect.poll(titles).toEqual([`First ${box.prefix}`, `Second ${box.prefix}`]);
    await bar.getByRole('link', { name: `First ${box.prefix}` }).click();
    await inTab('Third');
    await expect.poll(titles).toEqual([`First ${box.prefix}`, `Third ${box.prefix}`, `Second ${box.prefix}`]);

    // Third shown, Fourth opened replaces it; a tab dragged onto the first takes its place.
    await page.goto(`/notes/${fourth.id}`);
    await expect.poll(titles).toEqual([`First ${box.prefix}`, `Fourth ${box.prefix}`, `Second ${box.prefix}`]);
    await bar
      .getByRole('link', { name: `Second ${box.prefix}` })
      .dragTo(bar.getByRole('link', { name: `First ${box.prefix}` }));
    await expect.poll(titles).toEqual([`Second ${box.prefix}`, `First ${box.prefix}`, `Fourth ${box.prefix}`]);

    // A note trashed from elsewhere loses its tab once opened: the API no longer finds it.
    await box.api.delete(`/api/notes/${second.id}`);
    await bar.getByRole('link', { name: `Second ${box.prefix}` }).click();
    await expect.poll(titles).toEqual([`First ${box.prefix}`, `Fourth ${box.prefix}`]);

    // The open note's tab closed opens its neighbour, the one before it: alone, it shows no bar.
    await bar.getByRole('link', { name: `Fourth ${box.prefix}` }).click();
    await bar.getByRole('button', { name: `Fermer l'onglet Fourth ${box.prefix}` }).click();
    await expect(page).toHaveURL(`/notes/${first.id}`);
    await expect(bar).toHaveCount(0);

    // The others closed keep the one right-clicked, opened.
    await inTab('Third');
    await bar.getByRole('link', { name: `First ${box.prefix}` }).click();
    await inTab('Fourth');
    await expect(bar.getByRole('link')).toHaveCount(3);
    await bar.getByRole('link', { name: `Third ${box.prefix}` }).click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Fermer les autres onglets' }).click();
    await expect(page).toHaveURL(`/notes/${third.id}`);
    await expect(bar).toHaveCount(0);

    // What is left survives a reload; all of them closed go home.
    await page.goto(`/notes/${first.id}`);
    await inTab('Fourth');
    await expect(bar.getByRole('link')).toHaveCount(2);
    await page.reload();
    await expect.poll(titles).toEqual([`First ${box.prefix}`, `Fourth ${box.prefix}`]);
    await bar.getByRole('button', { name: 'Fermer tous les onglets' }).click();
    await expect(page).toHaveURL('/');
    await page.goto(`/notes/${first.id}`);
    await inTab('Fourth');
    await expect(bar).toBeVisible();

    // A lower plan takes the bar away.
    await box.api.patch('/api/me', { data: { plan: 'pro' } });
    await page.reload();
    await expect(page.getByRole('textbox')).toBeVisible();
    await expect(bar).toHaveCount(0);
  } finally {
    await box.api.patch('/api/me', { data: { plan: 'base' } });
    await box.api.delete(`/api/trash/${second.id}`);
  }
});

// The Ultra split view (#128): ⌘ + click on a [[link]] opens its note beside, the context panel gives way, both
// editors save, the panel survives a reload and closes by its button; a right click or ⌘ + click on a tab, the ⌘ K
// palette and a right click on the link open it beside again; on a phone the two notes stack.
test('the Ultra split view edits two notes side by side', async ({ page }) => {
  const beside = await box.note(`# Beside ${box.prefix}\n\nAutre`);
  const main = await box.note(`# Main ${box.prefix}\n\nVoir [Beside ${box.prefix}](note:${beside.id})`);
  const panel = page.getByRole('region', { name: 'Note à côté' });
  try {
    await box.api.patch('/api/me', { data: { plan: 'ultra' } });
    // Both notes in tabs: the one beside asked for in a tab of its own.
    await page.goto(`/notes/${main.id}`);
    const mainEditor = page.locator(`[data-note-id="${main.id}"] .ProseMirror`);
    await mainEditor.locator('a[data-wikilink]').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
    await expect(page.getByRole('textbox').getByText('Autre')).toBeVisible();
    await page
      .getByRole('navigation', { name: 'Notes ouvertes' })
      .getByRole('link', { name: `Main ${box.prefix}` })
      .click();
    await expect(page).toHaveURL(`/notes/${main.id}`);
    await expect(page.locator('#context-panel')).toHaveCount(1);
    await mainEditor.locator('a[data-wikilink]').click({ modifiers: ['ControlOrMeta'] });

    await expect(page).toHaveURL(`/notes/${main.id}`);
    const besideEditor = panel.locator('.ProseMirror');
    await expect(besideEditor.getByText('Autre')).toBeVisible();
    await expect(page.locator('#context-panel')).toHaveCount(0);

    await cursorAtEnd(mainEditor, 'Voir');
    await page.keyboard.type(' ici');
    await cursorAtEnd(besideEditor, 'Autre');
    await page.keyboard.type(' là');
    await expect.poll(() => box.content(main.id), { timeout: 5000 }).toContain(') ici');
    await expect.poll(() => box.content(beside.id), { timeout: 5000 }).toBe(`# Beside ${box.prefix}\n\nAutre là`);

    await page.reload();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await panel.getByRole('button', { name: 'Fermer la note à côté' }).click();
    await expect(panel).toHaveCount(0);
    await expect(page.locator('#context-panel')).toHaveCount(1);

    const bar = page.getByRole('navigation', { name: 'Notes ouvertes' });
    await bar.getByRole('link', { name: `Beside ${box.prefix}` }).click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir à côté' }).click();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);

    // ⌘ K: "Open a note beside…" turns the palette into a pick of the note, by the same search.
    await panel.getByRole('button', { name: 'Fermer la note à côté' }).click();
    await page.keyboard.press('ControlOrMeta+k');
    await page.getByRole('option', { name: 'Ouvrir une note à côté…' }).click();
    await page.getByPlaceholder('Chercher une note, une action…').fill(`Beside ${box.prefix}`);
    await page.getByRole('option', { name: `Beside ${box.prefix}` }).click();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);

    // ⌘ + click on a tab opens it beside too, not in a new tab of the browser.
    await panel.getByRole('button', { name: 'Fermer la note à côté' }).click();
    await bar.getByRole('link', { name: `Beside ${box.prefix}` }).click({ modifiers: ['ControlOrMeta'] });
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);
    expect(page.context().pages()).toHaveLength(1);

    // A right click on the [[link]] itself opens it beside.
    await panel.getByRole('button', { name: 'Fermer la note à côté' }).click();
    await mainEditor.locator('a[data-wikilink]').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir à côté' }).click();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);

    // On a phone the two notes stack, the one beside under the other.
    await page.setViewportSize({ width: 390, height: 844 });
    const [top, under] = await Promise.all([mainEditor.boundingBox(), besideEditor.boundingBox()]);
    expect(under!.y).toBeGreaterThan(top!.y);
    expect(under!.x).toBeLessThan(top!.x + top!.width);
  } finally {
    await box.api.patch('/api/me', { data: { plan: 'base' } });
  }
});
