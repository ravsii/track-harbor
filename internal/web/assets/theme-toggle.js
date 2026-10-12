// Importable behavior for an icon button containing an SVG <use> element.
export function mountThemeToggle(button) {
  const key = 'trackharbor-theme';
  const media = matchMedia('(prefers-color-scheme: dark)');
  let choice = document.documentElement.dataset.theme || null;

  function activeTheme() {
    return choice || (media.matches ? 'dark' : 'light');
  }

  function update() {
    if (choice) document.documentElement.dataset.theme = choice;
    else document.documentElement.removeAttribute('data-theme');
    const active = activeTheme();
    const next = active === 'dark' ? 'light' : 'dark';
    const label = `Theme: ${active}. Switch to ${next} theme`;
    button.setAttribute('aria-label', label);
    button.title = label;
    button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${active === 'dark' ? 'moon-01' : 'sun-01'}`);
  }

  button.addEventListener('click', () => {
    choice = activeTheme() === 'dark' ? 'light' : 'dark';
    try {
      localStorage.setItem(key, choice);
    } catch (_) {
      // The current page still remembers the choice if storage is unavailable.
    }
    update();
  });
  media.addEventListener('change', update);
  window.addEventListener('storage', event => {
    if (event.key !== key && event.key !== null) return;
    choice = event.newValue === 'light' || event.newValue === 'dark' ? event.newValue : null;
    update();
  });
  update();
}
