export default defineAppConfig({
  ui: {
    // The theme: each key takes a Tailwind palette or one declared in main.css.
    colors: { primary: 'notes', secondary: 'slate', neutral: 'plain' },
    // Empty states sit in the page, not in a bordered box, in the middle of the space under the page header.
    // TODO: 12rem is the layout padding plus the page header, adjust it if the header grows.
    empty: { slots: { root: 'min-h-[calc(100dvh-12rem)]' }, defaultVariants: { variant: 'naked' } },
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
