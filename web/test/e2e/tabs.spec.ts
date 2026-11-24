import { expect, test, type Page } from '@playwright/test';
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

// The Ultra panels (#128, #253): ⌘ + click on a [[link]] opens its note in a second panel, the context panel gives way,
// both editors save, the panels survive a reload and the last tab of one closed closes it; a right click or ⌘ + click
// on a tab, the ⌘ K palette and a right click on the link open it beside again; on a phone the two notes stack.
test('the Ultra panels edit two notes side by side', async ({ page }) => {
  const beside = await box.note(`# Beside ${box.prefix}\n\nAutre`);
  const main = await box.note(`# Main ${box.prefix}\n\nVoir [Beside ${box.prefix}](note:${beside.id})`);
  const panel = page.getByRole('region', { name: 'Panneau 2' });
  const closeBeside = () => panel.getByRole('button', { name: `Fermer l'onglet Beside ${box.prefix}` }).click();
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
    // Typing in the second panel made it the current one: the address follows it.
    await expect(page).toHaveURL(`/notes/${beside.id}`);

    await page.reload();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await closeBeside();
    await expect(panel).toHaveCount(0);
    await expect(page).toHaveURL(`/notes/${main.id}`);
    await expect(page.locator('#context-panel')).toHaveCount(1);

    // The note beside took its tab with it: closed, it has none, a tab of its own is asked for again.
    const bar = page.getByRole('navigation', { name: 'Notes ouvertes' });
    const tabAgain = async () => {
      await mainEditor.locator('a[data-wikilink]').click({ button: 'right' });
      await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
      await expect(page).toHaveURL(`/notes/${beside.id}`);
      await expect(page.getByRole('textbox').getByText('Autre là')).toBeVisible();
      await bar.getByRole('link', { name: `Main ${box.prefix}` }).click();
      await expect(page).toHaveURL(`/notes/${main.id}`);
    };
    await tabAgain();
    await bar.getByRole('link', { name: `Beside ${box.prefix}` }).click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir à côté' }).click();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);

    // ⌘ K: "Open a note beside…" turns the palette into a pick of the note, by the same search.
    await closeBeside();
    await page.keyboard.press('ControlOrMeta+k');
    await page.getByRole('option', { name: 'Ouvrir une note à côté…' }).click();
    await page.getByPlaceholder('Chercher une note, une action…').fill(`Beside ${box.prefix}`);
    await page.getByRole('option', { name: `Beside ${box.prefix}` }).click();
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);

    // ⌘ + click on a tab opens it beside too, not in a new tab of the browser.
    await closeBeside();
    await tabAgain();
    await bar.getByRole('link', { name: `Beside ${box.prefix}` }).click({ modifiers: ['ControlOrMeta'] });
    await expect(besideEditor.getByText('Autre là')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${main.id}`);
    expect(page.context().pages()).toHaveLength(1);

    // A right click on the [[link]] itself opens it beside.
    await closeBeside();
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

// The tabs of each panel (#242, #253): a note beside is a tab of the second panel's bar, a [[link]] of a panel opens
// there, what is typed is saved when another tab is shown, a note opened in one panel takes its tab from the other
// (never in both), a tab dragged moves between the bars or opens a panel on an edge, a reload keeps them, a wider
// screen holds a third panel and a narrower one folds it into the second.
async function ultraSplit(page: Page) {
  const fourth = await box.note(`# Fourth ${box.prefix}`);
  const third = await box.note(`# Third ${box.prefix}\n\n[Fourth](note:${fourth.id})`);
  const second = await box.note(
    `# Second ${box.prefix}\n\n[Third](note:${third.id}) [Fourth](note:${fourth.id}) Texte`,
  );
  const extra = await box.note(`# Extra ${box.prefix}`);
  const first = await box.note(
    `# First ${box.prefix}\n\n[Second](note:${second.id}) [Third](note:${third.id}) [Extra](note:${extra.id})`,
  );
  const panel = page.getByRole('region', { name: 'Panneau 2' });
  const right = panel.getByRole('navigation', { name: 'Notes ouvertes' });
  const left = page.getByRole('region', { name: 'Panneau 1' }).getByRole('navigation', { name: 'Notes ouvertes' });
  const title = (name: string) => `${name} ${box.prefix}`;
  await box.api.patch('/api/me', { data: { plan: 'ultra' } });
  await page.goto(`/notes/${first.id}`);
  await expect(page.getByRole('textbox').getByRole('heading', { name: title('First') })).toBeVisible();
  return {
    first,
    second,
    third,
    fourth,
    extra,
    panel,
    right,
    left,
    title,
    rightTitles: () => right.getByRole('link').allTextContents(),
    leftTitles: () => left.getByRole('link').allTextContents(),
    link: (name: string) => page.locator(`[data-note-id="${first.id}"] a[data-wikilink]`, { hasText: name }),
    inPanel: (name: string) => panel.locator('a[data-wikilink]', { hasText: name }),
    heading: (name: string) => panel.getByRole('textbox').getByRole('heading', { name: title(name) }),
    /** Second beside, Third in a tab of its own asked from the panel: the right bar [Second, Third]. */
    async twoBeside() {
      await this.link('Second').click({ modifiers: ['ControlOrMeta'] });
      await expect(this.heading('Second')).toBeVisible();
      await this.inPanel('Third').click({ button: 'right' });
      await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
      await expect.poll(this.rightTitles).toEqual([title('Second'), title('Third')]);
    },
  };
}
const endPlan = () => box.api.patch('/api/me', { data: { plan: 'base' } });

test('each panel has its bar and opens its links there', async ({ page }) => {
  const s = await ultraSplit(page);
  try {
    // Two panels: each shows its bar, even with a single tab.
    await s.link('Second').click({ modifiers: ['ControlOrMeta'] });
    await expect(s.heading('Second')).toBeVisible();
    await expect.poll(s.rightTitles).toEqual([s.title('Second')]);
    await expect.poll(s.leftTitles).toEqual([s.title('First')]);
    await s.inPanel('Third').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
    await expect.poll(s.rightTitles).toEqual([s.title('Second'), s.title('Third')]);
    // Right-clicked, the second panel became the current one: the address shows its note.
    await expect(page).toHaveURL(`/notes/${s.third.id}`);

    // A plain click on a [[link]] of the second panel opens its note there, which becomes the current one.
    await s.inPanel('Fourth').click();
    await expect(s.heading('Fourth')).toBeVisible();
    await expect(page).toHaveURL(`/notes/${s.fourth.id}`);
    await expect.poll(s.rightTitles).toContain(s.title('Fourth'));
    await expect.poll(s.leftTitles).toEqual([s.title('First')]);
  } finally {
    await endPlan();
  }
});

test('a panel saves what is typed when another of its tabs is shown', async ({ page }) => {
  const s = await ultraSplit(page);
  try {
    await s.twoBeside();
    await s.right.getByRole('link', { name: s.title('Second') }).click();
    await expect(s.heading('Second')).toBeVisible();
    await cursorAtEnd(s.panel.locator('.ProseMirror'), 'Texte');
    await page.keyboard.type(' saisi');
    await s.right.getByRole('link', { name: s.title('Third') }).click();
    await expect(s.heading('Third')).toBeVisible();
    await page.reload();
    await expect.poll(() => box.content(s.second.id)).toContain('Texte saisi');
  } finally {
    await endPlan();
  }
});

test('a note shown in one panel takes the focus there, a hidden tab of it moves, never in both', async ({ page }) => {
  const s = await ultraSplit(page);
  try {
    await s.twoBeside();
    // Third shown on the right: a click on its link on the left goes there.
    await s.link('Third').click();
    await expect(page).toHaveURL(`/notes/${s.third.id}`);
    await expect.poll(s.rightTitles).toEqual([s.title('Second'), s.title('Third')]);
    await expect(page.locator(`[data-note-id="${s.third.id}"] .ProseMirror`)).toHaveCount(1);
    // Third in a tab behind Second: a click on its link on the left opens it on the left, its tab with it.
    await s.right.getByRole('link', { name: s.title('Second') }).click();
    await expect(s.heading('Second')).toBeVisible();
    await s.link('Third').click();
    await expect(page).toHaveURL(`/notes/${s.third.id}`);
    await expect.poll(s.rightTitles).toEqual([s.title('Second')]);
    await expect.poll(s.leftTitles).toContain(s.title('Third'));
    await expect(page.locator(`[data-note-id="${s.third.id}"] .ProseMirror`)).toHaveCount(1);
  } finally {
    await endPlan();
  }
});

test('a tab dragged onto the other bar changes panel, the tabs survive a reload', async ({ page }) => {
  const s = await ultraSplit(page);
  try {
    await s.twoBeside();
    await s.inPanel('Fourth').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
    await expect.poll(s.rightTitles).toEqual([s.title('Second'), s.title('Third'), s.title('Fourth')]);
    await s.link('Extra').click({ button: 'right' });
    await page.getByRole('menuitem', { name: 'Ouvrir dans un nouvel onglet' }).click();
    await expect(page).toHaveURL(`/notes/${s.extra.id}`);
    await expect.poll(s.leftTitles).toEqual([s.title('First'), s.title('Extra')]);
    await s.left.getByRole('link', { name: s.title('First') }).click();

    // To the first panel: the note opens there, its tab leaves the right bar.
    await s.right
      .getByRole('link', { name: s.title('Fourth') })
      .dragTo(s.left.getByRole('link', { name: s.title('Extra') }));
    await expect.poll(s.leftTitles).toContain(s.title('Fourth'));
    await expect.poll(s.rightTitles).toEqual([s.title('Second'), s.title('Third')]);

    // To the second panel, the other way.
    await s.left
      .getByRole('link', { name: s.title('Extra') })
      .dragTo(s.right.getByRole('link', { name: s.title('Second') }));
    await expect.poll(s.rightTitles).toContain(s.title('Extra'));
    await expect.poll(s.leftTitles).not.toContain(s.title('Extra'));

    // A reload keeps the tabs of both panels and the note each shows.
    const active = await s.right.locator('a[aria-current]').textContent();
    const tabs = await s.rightTitles();
    expect(tabs).toHaveLength(3);
    await page.reload();
    await expect.poll(s.rightTitles).toEqual(tabs);
    await expect(s.right.locator('a[aria-current]')).toHaveText(active!);

    // The last tab of the second panel closed closes it.
    for (const tab of tabs) await s.right.getByRole('button', { name: `Fermer l'onglet ${tab}` }).click();
    await expect(s.panel).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Panneau 1' })).toBeVisible();
  } finally {
    await endPlan();
  }
});

test('a wide screen holds a third panel, opened on an edge, a narrower one folds it', async ({ page }) => {
  await page.setViewportSize({ width: 1800, height: 900 });
  const s = await ultraSplit(page);
  const third = page.getByRole('region', { name: 'Panneau 3' });
  try {
    await s.twoBeside();
    // A tab dropped on the right edge of the second panel opens a third panel.
    // The edges show once the drag started: the mouse by hand, not dragTo, which looks for its target first.
    await s.right.getByRole('link', { name: s.title('Third') }).hover();
    await page.mouse.down();
    await page.mouse.move(0, 0, { steps: 5 });
    const edge = s.panel.locator('[data-edge="after"]');
    await expect(edge).toBeVisible();
    const box = (await edge.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 5 });
    await page.mouse.up();
    await expect(third.getByRole('textbox').getByRole('heading', { name: s.title('Third') })).toBeVisible();
    await expect.poll(s.rightTitles).toEqual([s.title('Second')]);

    // ⌘ + click from the third, the last panel with no room left: the note opens in the one before it.
    await third.locator('a[data-wikilink]', { hasText: 'Fourth' }).click({ modifiers: ['ControlOrMeta'] });
    await expect(s.heading('Fourth')).toBeVisible();

    // Narrower, the third panel's tabs go to the second, nothing lost.
    await page.setViewportSize({ width: 1280, height: 900 });
    await expect(third).toHaveCount(0);
    await expect.poll(s.rightTitles).toEqual([s.title('Fourth'), s.title('Third')]);
  } finally {
    await endPlan();
  }
});
