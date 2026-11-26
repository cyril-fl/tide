import { expect, test } from '@playwright/test';
import { sandbox } from './helpers';

let box: Awaited<ReturnType<typeof sandbox>>;
test.beforeEach(async () => (box = await sandbox()));
test.afterEach(async () => box.dispose());

// The Pro preview of the lists (#256): a click on a note shows it read only beside the folder, a [[link]] in it shows
// its note, ⌘-click still selects, a double click opens the note in the editor; the choice survives a
// reload, the search's list has it too, and below 1024px a click opens the note as before.
test('the Pro preview shows a note read only beside its list', async ({ page }) => {
  const other = await box.note(`# Other ${box.prefix}\n\nAilleurs`);
  const first = await box.note(`# First ${box.prefix}\n\nPremier texte [Other ${box.prefix}](note:${other.id})`);
  const preview = page.getByRole('region', { name: 'Aperçu' });
  const row = (title: string) => page.locator('[data-id]').getByRole('link', { name: `${title} ${box.prefix}` });
  try {
    await box.api.patch('/api/me', { data: { plan: 'pro' } });
    await page.goto(`/folders/${box.folder.id}`);
    await page.getByRole('button', { name: 'Aperçu' }).click();
    await expect(preview.getByText('Cliquez sur une note pour la lire ici')).toBeVisible();

    await row('First').click();
    await expect(preview.getByRole('heading', { name: `First ${box.prefix}` })).toBeVisible();
    await expect(preview.getByText('Premier texte')).toBeVisible();
    await expect(page).toHaveURL(`/folders/${box.folder.id}`);
    await expect(preview.locator('[contenteditable="true"]')).toHaveCount(0);

    await preview.locator('a[data-wikilink]').click();
    await expect(preview.getByText('Ailleurs')).toBeVisible();

    // ⌘-click selects, the preview stays.
    await row('First').click({ modifiers: ['ControlOrMeta'] });
    await expect(page.getByText('1 sélectionné')).toBeVisible();
    await expect(preview.getByText('Ailleurs')).toBeVisible();
    await page.keyboard.press('Escape');

    await page.locator(`[data-id="${other.id}"] a`).dblclick();
    await expect(page).toHaveURL(`/notes/${other.id}`);
    await expect(page.getByRole('textbox').getByText('Ailleurs')).toBeVisible();

    // Kept across a reload; a double click opens the note.
    await page.goto(`/folders/${box.folder.id}`);
    await expect(page.getByRole('button', { name: 'Aperçu' })).toHaveAttribute('aria-pressed', 'true');
    await row('First').dblclick();
    await expect(page).toHaveURL(`/notes/${first.id}`);

    await page.goto(`/search?q=${box.prefix}`);
    await page.locator(`[data-id="${other.id}"] a`).click();
    await expect(preview.getByText('Ailleurs')).toBeVisible();

    // A phone: no preview, the note opens.
    await page.setViewportSize({ width: 800, height: 900 });
    await page.goto(`/folders/${box.folder.id}`);
    await expect(page.getByRole('button', { name: 'Aperçu' })).toHaveCount(0);
    await row('First').click();
    await expect(page).toHaveURL(`/notes/${first.id}`);
  } finally {
    await box.api.patch('/api/me', { data: { plan: 'base' } });
  }
});

// Unlocked, the preview writes in place (the grid stays offered beside it); locked again or at the next note, it
// reads; a note locked has no padlock to open.
test('the Pro preview unlocks a note to edit it in place', async ({ page }) => {
  const note = await box.note(`# Quick ${box.prefix}\n\nAvant`);
  const locked = await box.note(`# Locked ${box.prefix}\n\nFigé`);
  await box.api.patch(`/api/notes/${locked.id}`, { data: { readonly: true } });
  const preview = page.getByRole('region', { name: 'Aperçu' });
  const row = (title: string) => page.locator('[data-id]').getByRole('link', { name: `${title} ${box.prefix}` });
  try {
    await box.api.patch('/api/me', { data: { plan: 'pro' } });
    await page.goto(`/folders/${box.folder.id}`);
    const toggle = page.getByRole('button', { name: 'Aperçu' });
    if ((await toggle.getAttribute('aria-pressed')) !== 'true') await toggle.click();
    await expect(page.getByRole('button', { name: /Vue (mosaïque|liste)/ })).toBeVisible();

    await row('Quick').click();
    await expect(preview.locator('[contenteditable="true"]')).toHaveCount(0);
    await preview.getByRole('button', { name: 'Modifier ici' }).click();
    const editor = preview.locator('[contenteditable="true"]');
    await editor.getByText('Avant').click();
    const saved = page.waitForResponse(
      (r) => r.url().endsWith(`/api/notes/${note.id}`) && r.request().method() === 'PATCH',
    );
    await page.keyboard.press('End');
    await page.keyboard.type(' Après');
    await saved;
    await preview.getByRole('button', { name: 'Revenir à la lecture' }).click();
    await expect(preview.locator('[contenteditable="true"]')).toHaveCount(0);
    await expect(preview.getByText('Avant Après')).toBeVisible();

    await row('Locked').click();
    await expect(preview.getByText('Figé')).toBeVisible();
    await expect(preview.getByRole('button', { name: 'Modifier ici' })).toHaveCount(0);
  } finally {
    await box.api.patch('/api/me', { data: { plan: 'base' } });
  }
});
