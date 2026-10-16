(() => {
  const navigation = document.getElementById('sidebar-navigation');
  const narrow = matchMedia('(max-width: 700px)');
  const setNavigation = () => { if (navigation) navigation.open = !narrow.matches; };
  narrow.addEventListener('change', setNavigation);
  setNavigation();

  function pageChanged() {
    const name = document.querySelector('#content h1')?.textContent;
    if (name) {
      document.title = `TrackHarbor · ${name}`;
      const label = document.getElementById('sidebar-current-page');
      if (label) label.textContent = name;
    }
    setNavigation();
    for (const id of ['sync-state', 'connection-state']) sourceFeedback(document.getElementById(id));
  }

  function sourceFeedback(field) {
    if (!field) return;
    const error = /^(Sync failed|Sync timed out|Library synced, but|Token rejected|Could not)/.test(field.textContent);
    field.dataset.error = String(error);
    field.setAttribute('role', error ? 'alert' : 'status');
  }

  document.addEventListener('click', event => {
    const button = event.target.closest('[data-password-toggle]');
    if (!button) return;
    const editor = document.getElementById(button.getAttribute('aria-controls'));
    if (!editor) return;
    editor.hidden = !editor.hidden;
    button.setAttribute('aria-expanded', String(!editor.hidden));
    if (!editor.hidden) editor.querySelector('input')?.focus();
  });

  document.addEventListener('htmx:beforeRequest', event => {
    const form = event.detail.elt.closest('form[data-feedback]');
    const feedback = document.getElementById(form?.dataset.feedback);
    if (!feedback) return;
    feedback.textContent = 'Working…';
    feedback.dataset.error = 'false';
    feedback.setAttribute('role', 'status');
    for (const field of form.querySelectorAll('[aria-invalid]')) field.removeAttribute('aria-invalid');
  });
  for (const name of ['htmx:responseError', 'htmx:sendError']) {
    document.addEventListener(name, event => {
      const form = event.detail.elt.closest('form[data-feedback]');
      const feedback = document.getElementById(form?.dataset.feedback);
      if (!feedback) return;
      feedback.textContent = event.detail.xhr?.responseText || 'Could not complete this action. Check your connection and try again.';
      feedback.dataset.error = 'true';
      feedback.setAttribute('role', 'alert');
      if (event.detail.xhr?.status === 400) {
        const field = form.querySelector('textarea, input:not([type="hidden"])');
        field?.setAttribute('aria-invalid', 'true');
        field?.setAttribute('aria-describedby', feedback.id);
      }
    });
  }
  document.addEventListener('htmx:afterSwap', event => {
    if (event.detail.target.id === 'content') pageChanged();
    if (['sync-state', 'connection-state'].includes(event.detail.target.id)) sourceFeedback(event.detail.target);
  });
  document.addEventListener('htmx:historyRestore', pageChanged);
  pageChanged();
})();
