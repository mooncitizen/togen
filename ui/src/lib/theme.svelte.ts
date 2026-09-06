import { getContext, setContext } from 'svelte';

import type { ThemeSetting } from './types.ts';

export type Scheme = 'dark' | 'light';

const remembered = 'togen.theme';
const prefersDark = '(prefers-color-scheme: dark)';

// The project's setting comes from togen.yml and the toggle overrides it for
// this browser session; nothing is written back to the file.
export class Theme {
  #read: () => ThemeSetting = () => 'dark';

  override = $state.raw<Scheme | null>(null);
  system = $state.raw<Scheme>('dark');

  readonly setting: ThemeSetting = $derived(this.#read());
  readonly scheme: Scheme = $derived(
    this.override ?? (this.setting === 'system' ? this.system : this.setting),
  );

  constructor(read: () => ThemeSetting) {
    this.#read = read;
    this.override = recall();
  }

  toggle(): void {
    this.override = this.scheme === 'dark' ? 'light' : 'dark';
    remember(this.override);
  }

  // Sets data-theme on the document and follows the OS until the returned
  // function is called.
  attach(): () => void {
    const media = window.matchMedia(prefersDark);
    const follow = () => {
      this.system = media.matches ? 'dark' : 'light';
    };
    follow();
    media.addEventListener('change', follow);
    const stop = $effect.root(() => {
      $effect(() => {
        document.documentElement.dataset.theme = this.scheme;
      });
    });
    return () => {
      media.removeEventListener('change', follow);
      stop();
    };
  }
}

const key = Symbol('togen.theme');

export function setTheme(theme: Theme): void {
  setContext(key, theme);
}

export function getTheme(): Theme {
  return getContext<Theme>(key);
}

// Session storage is a convenience, not a requirement: a browser that refuses
// it just forgets the toggle on reload.
function recall(): Scheme | null {
  try {
    const held = window.sessionStorage.getItem(remembered);
    return held === 'dark' || held === 'light' ? held : null;
  } catch {
    return null;
  }
}

function remember(scheme: Scheme): void {
  try {
    window.sessionStorage.setItem(remembered, scheme);
  } catch {
    return;
  }
}
