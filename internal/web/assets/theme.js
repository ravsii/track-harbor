(() => {
  const key = 'trackharbor-theme';
  let choice = 'system';
  try {
    const saved = localStorage.getItem(key);
    if (saved === 'light' || saved === 'dark') choice = saved;
  } catch (_) {
    // Private browsing can disable local storage; the selector still works.
  }

  function apply(value) {
    if (value === 'system') document.documentElement.removeAttribute('data-theme');
    else document.documentElement.dataset.theme = value;
  }

  apply(choice);
  document.addEventListener('DOMContentLoaded', () => {
    const topbar = document.getElementById('music-player');
    if (topbar) {
      const updateHeight = () => document.documentElement.style.setProperty('--topbar-height', `${topbar.getBoundingClientRect().height}px`);
      updateHeight();
      new ResizeObserver(updateHeight).observe(topbar);
    }
    const button = document.getElementById('theme-choice');
    if (!button) return;
    import('/assets/theme-toggle.js').then(({ mountThemeToggle }) => mountThemeToggle(button));
  });
})();
