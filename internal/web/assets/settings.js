(() => {
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (form.id !== 'settings-form' && form.id !== 'personal-proxy-form' && !form.classList.contains('proxy-delete')) return;
    event.preventDefault();
    const feedback = document.getElementById(form.id === 'settings-form' ? 'settings-feedback' : 'personal-proxy-feedback');
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    feedback.textContent = form.id === 'personal-proxy-form' ? 'Checking YouTube through proxy…' : 'Saving settings…';
    feedback.dataset.error = 'false';
    try {
      const response = await fetch(form.action, {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: new URLSearchParams(new FormData(form))
      });
      if (!response.ok) {
        feedback.textContent = (await response.text()).trim() || 'Could not update settings.';
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
  document.addEventListener('click', async event => {
    const button = event.target.closest('.proxy-reveal');
    if (!button) return;
    const row = button.closest('[data-proxy-id]');
    const output = row.querySelector('.proxy-credentials');
    if (button.getAttribute('aria-expanded') === 'true') {
      output.textContent = '';
      output.hidden = true;
      button.setAttribute('aria-expanded', 'false');
      button.textContent = 'Show credentials';
      return;
    }
    button.disabled = true;
    try {
      const response = await fetch(`/settings/personal/${row.dataset.proxyId}/credentials`, {cache: 'no-store'});
      if (!response.ok) throw new Error();
      const credentials = await response.json();
      if (!row.isConnected) return;
      output.textContent = `Login: ${credentials.login || '—'} · Password: ${credentials.password || '—'}`;
      output.hidden = false;
      button.setAttribute('aria-expanded', 'true');
      button.textContent = 'Hide credentials';
    } catch (_) {
      const feedback = document.getElementById('personal-proxy-feedback');
      if (feedback) {
        feedback.textContent = 'Could not show proxy credentials.';
        feedback.dataset.error = 'true';
      }
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });
})();
