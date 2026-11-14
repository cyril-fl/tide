export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  // The API runs on the loopback and answers the browser directly: no server side here.
  ssr: false,
  modules: ['@nuxt/eslint', '@nuxt/ui', '@vueuse/nuxt', '@nuxtjs/i18n'],
  i18n: {
    strategy: 'no_prefix',
    defaultLocale: 'fr',
    locales: [
      { code: 'fr', file: 'fr.json' },
      { code: 'en', file: 'en.json' },
    ],
    // The cookie keeps the last language chosen: the login page, without an account, shows it too.
    detectBrowserLanguage: { useCookie: true, cookieKey: 'notes-locale' },
  },
  css: ['~/assets/css/main.css'],
  // prosemirror-search has its own prosemirror-* dependencies: a second copy breaks the decorations of the editor.
  vite: { resolve: { dedupe: ['prosemirror-model', 'prosemirror-state', 'prosemirror-view'] } },
  runtimeConfig: {
    public: { apiBase: 'http://localhost:8080' },
  },
});
