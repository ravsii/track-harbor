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
    const button = document.getElementById('theme-choice');
    if (!button) return;
    const modes = ['system', 'light', 'dark'];
    const icons = ['computer', 'sun-01', 'moon-01'];
    function updateButton() {
      const label = `Theme: ${choice}. Click for ${modes[(modes.indexOf(choice) + 1) % modes.length]}`;
      button.setAttribute('aria-label', label);
      button.title = label;
      button.querySelector('use').setAttribute('href', `/assets/icons.svg#hg-${icons[modes.indexOf(choice)]}`);
    }
    updateButton();
    button.addEventListener('click', () => {
      choice = modes[(modes.indexOf(choice) + 1) % modes.length];
      apply(choice);
      try {
        if (choice === 'system') localStorage.removeItem(key);
        else localStorage.setItem(key, choice);
      } catch (_) {
        // Keep the selected theme active for this page even without storage.
      }
      updateButton();
    });
  });
})();
