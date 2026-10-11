(() => {
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (form.id !== 'settings-form') return;
    event.preventDefault();
    const feedback = document.getElementById('settings-feedback');
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    feedback.textContent = 'Saving settings…';
    feedback.dataset.error = 'false';
    try {
      const response = await fetch(form.action, {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: new URLSearchParams(new FormData(form))
      });
      if (!response.ok) {
        feedback.textContent = (await response.text()).trim() || 'Could not save settings.';
        feedback.dataset.error = 'true';
        return;
      }
      htmx.ajax('GET', '/?source=settings&saved=1', {target: '#content', select: '#content', swap: 'outerHTML'});
    } catch (_) {
      feedback.textContent = 'Could not save settings. Try again.';
      feedback.dataset.error = 'true';
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });
})();
