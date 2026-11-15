// The themes an account picks from: the Nuxt UI colors they set, palettes of Tailwind or of main.css.
// Their ids are repeated in domain.Themes (checked by its users_test.go), the API refuses the others.
const own = (id: string) => ({
  id,
  primary: id,
  neutral: `${id}-gray`,
  success: `${id}-success`,
  info: `${id}-info`,
  warning: `${id}-warning`,
  error: `${id}-error`,
});

export const themes = [
  { id: 'notes', primary: 'notes', neutral: 'plain', success: 'green', info: 'blue', warning: 'yellow', error: 'red' },
  own('dracula'),
  own('catppuccin'),
  own('nord'),
  own('gruvbox'),
  own('everforest'),
  own('tokyo'),
  own('vue'),
];

export type Theme = (typeof themes)[number];
