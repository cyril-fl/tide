/**
 * The subsection of each feature of the app in the settings' comparison. Which plan has a feature is the API's alone:
 * domain.Features of notes-go, whose test checks the names match; the account answers its own (User.features).
 * implied: left out of the table, it comes with a feature already listed (the " / " menu, the [[ links, the Data section).
 */
const groups = {
  graph: 'navigation',
  tabs: 'display',
  splitView: 'display',
  focusMode: 'display',
  contextPanel: 'display',
  settingsNav: 'navigation',
  slashCommands: 'editing',
  tables: 'implied',
  codeHighlight: 'display',
  callouts: 'implied',
  wordCount: 'display',
  contextMenu: 'implied',
  paletteActions: 'navigation',
  sourceMode: 'editing',
  noteLock: 'editing',
  noteHistory: 'editing',
  brokenLinks: 'navigation',
  storageFiles: 'data',
  wikiLinks: 'editing',
  linkSuggestions: 'implied',
  noteEmbeds: 'implied',
  images: 'editing',
  attachments: 'editing',
  linkCards: 'editing',
  tagSuggestions: 'implied',
  snippets: 'editing',
  savedSearches: 'navigation',
  sidebarOrder: 'navigation',
  hideFolders: 'navigation',
  replaceEverywhere: 'editing',
  search: 'navigation',
  favorites: 'implied',
  backlinks: 'implied',
  themes: 'display',
  importExport: 'data',
  trash: 'data',
  storageUsage: 'implied',
} as const satisfies Record<string, 'display' | 'editing' | 'navigation' | 'data' | 'implied'>;

export type Feature = keyof typeof groups;
export const features = Object.keys(groups) as Feature[];

/** The features the settings compare, by subsection: offered by a plan, the others being off for everyone. */
export const listedFeatures = (offered: string[]) =>
  (['display', 'editing', 'navigation', 'data'] as const).map((group) => ({
    group,
    features: features.filter((feature) => groups[feature] === group && offered.includes(feature)),
  }));
