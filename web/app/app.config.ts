export default defineAppConfig({
  // The name of the app, in the tab title and on the login page.
  appName: 'Tide',
  ui: {
    // The theme: each key takes a Tailwind palette or one declared in main.css.
    colors: { primary: 'notes', secondary: 'slate', neutral: 'notes-gray' },
    // Empty states sit in the page, not in a bordered box, in the middle of the space under the page header.
    // TODO: 12rem is the layout padding plus the page header, adjust it if the header grows.
    empty: { slots: { root: 'min-h-[calc(100dvh-12rem)]' }, defaultVariants: { variant: 'naked' } },
    // Page headers: a compact title on one line, the actions beside it, the headline a muted line above.
    pageHeader: {
      slots: {
        root: 'mb-6 border-0 py-0',
        headline: 'mb-0 truncate text-xs font-normal text-muted',
        wrapper: 'flex-row items-start gap-4 lg:items-start',
        title: 'min-w-0 flex-1 truncate text-2xl font-semibold tracking-tight text-default sm:text-2xl',
        links: 'shrink-0 flex-nowrap gap-1',
      },
    },
    // Modals: one block without dividers, the page dimmed and blurred behind.
    modal: {
      slots: {
        overlay: 'backdrop-blur-[2px]',
        content: 'divide-y-0 dark:bg-elevated',
        header: 'min-h-0 px-5 pt-5 pb-0 sm:px-6',
        body: 'px-5 py-4 sm:px-6',
        footer: 'px-5 pt-2 pb-5 sm:px-6',
        title: 'text-base',
      },
      variants: {
        overlay: { true: { overlay: 'bg-black/30 dark:bg-black/50' } },
        fullscreen: { false: { content: 'rounded-xl shadow-2xl' } },
      },
    },
  },
});
