import { expect, test } from '@playwright/test';
import { cursorAtEnd, sandbox } from './helpers';

let box: Awaited<ReturnType<typeof sandbox>>;
test.beforeEach(async () => (box = await sandbox()));
test.afterEach(async () => box.dispose());

// The Ultra tabs of the editor (#128): a note opened takes one, a note no longer found loses its own, a tab closed
// opens its neighbour, they survive a reload and they go away with the Ultra plan.
test('the Ultra tabs keep the open notes', async ({ page }) => {
  const first = await box.note(`# First ${box.prefix}`);
  const second = await box.note(`# Second ${box.prefix}`);
  const third = await box.note(`# Third ${box.prefix}`);
  const bar = page.getByRole('navigation', { name: 'Notes ouvertes' });
  try {
    await box.api.patch('/api/me', { data: { plan: 'ultra' } });

    // One tab more per note opened: the tab is written once the editor is there, so each load is waited for.
    for (const [index, note] of [first, second, third].entries()) {
      await page.goto(`/notes/${note.id}`);
      await expect(bar.getByRole('link')).toHaveCount(index + 1);
    }

    // Back to the first note by its tab, the three still open.
    await bar.getByRole('link', { name: `First ${box.prefix}` }).click();
    await expect(page.getByRole('textbox').getByRole('heading', { name: `First ${box.prefix}` })).toBeVisible();
    await expect(bar.getByRole('link')).toHaveCount(3);

    // A note trashed from elsewhere loses its tab once opened: the API no longer finds it.
    await box.api.delete(`/api/notes/${second.id}`);
    await bar.getByRole('link', { name: `Second ${box.prefix}` }).click();
    await expect(bar.getByRole('link', { name: `Second ${box.prefix}` })).toHaveCount(0);
    await expect(bar.getByRole('link')).toHaveCount(2);

    // The open note's tab closed opens its neighbour, the one before it.
    await page.goto(`/notes/${third.id}`);
    await bar.getByRole('button', { name: `Fermer l'onglet Third ${box.prefix}` }).click();
    await expect(page).toHaveURL(`/notes/${first.id}`);

    // What is left survives a reload, and a lower plan takes the bar away.
    await page.reload();
    await expect(bar.getByRole('link', { name: `First ${box.prefix}` })).toBeVisible();
    await expect(bar.getByRole('link')).toHaveCount(1);

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
// editors save, the panel survives a reload and closes by its button; a right click on a tab opens it beside again.
test('the Ultra split view edits two notes side by side', async ({ page }) => {
  const beside = await box.note(`# Beside ${box.prefix}\n\nAutre`);
  const main = await box.note(`# Main ${box.prefix}\n\nVoir [Beside ${box.prefix}](note:${beside.id})`);
  const panel = page.getByRole('region', { name: 'Note à côté' });
  try {
    await box.api.patch('/api/me', { data: { plan: 'ultra' } });
    await page.goto(`/notes/${beside.id}`);
    await expect(page.getByRole('textbox').getByText('Autre')).toBeVisible();
    await page.goto(`/notes/${main.id}`);
    const mainEditor = page.locator(`[data-note-id="${main.id}"] .ProseMirror`);
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
  } finally {
    await box.api.patch('/api/me', { data: { plan: 'base' } });
  }
});
