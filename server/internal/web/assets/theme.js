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
    const selector = document.getElementById('theme-choice');
    if (!selector) return;
    selector.value = choice;
    selector.addEventListener('change', () => {
      choice = selector.value;
      apply(choice);
      try {
        if (choice === 'system') localStorage.removeItem(key);
        else localStorage.setItem(key, choice);
      } catch (_) {
        // Keep the selected theme active for this page even without storage.
      }
    });
  });
})();
