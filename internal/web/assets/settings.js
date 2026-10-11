(() => {
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (form.id !== 'settings-form' && form.id !== 'personal-proxy-form' && !form.classList.contains('proxy-delete')) return;
    event.preventDefault();
    const removing = form.classList.contains('proxy-delete');
    if (removing && !window.confirm(`Remove proxy ${form.closest('[data-proxy-id]').dataset.proxyHost}?`)) return;
    const feedback = document.getElementById(form.id === 'settings-form' ? 'settings-feedback' : 'personal-proxy-feedback');
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    feedback.textContent = form.id === 'personal-proxy-form' ? 'Checking YouTube through proxy…' : 'Saving settings…';
    feedback.dataset.error = 'false';
    try {
      const data = new URLSearchParams(new FormData(form));
      if (removing) data.set('confirm', 'yes');
      const response = await fetch(form.action, {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: data
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
    const host = row.dataset.proxyHost;
    const addressLabel = row.querySelector('.proxy-address');
    const label = button.querySelector('.proxy-reveal-label');
    const icon = button.querySelector('use');
    if (button.getAttribute('aria-expanded') === 'true') {
      addressLabel.textContent = addressLabel.dataset.masked;
      button.setAttribute('aria-expanded', 'false');
      label.textContent = 'Show URL';
      icon.setAttribute('href', '/assets/icons.svg#hg-view');
      button.setAttribute('aria-label', `Show proxy URL for ${host}`);
      return;
    }
    button.disabled = true;
    try {
      const response = await fetch(`/settings/personal/${row.dataset.proxyId}/address`, {cache: 'no-store'});
      if (!response.ok) throw new Error();
      const {address} = await response.json();
      if (!row.isConnected) return;
      addressLabel.textContent = address;
      button.setAttribute('aria-expanded', 'true');
      label.textContent = 'Hide URL';
      icon.setAttribute('href', '/assets/icons.svg#hg-view-off');
      button.setAttribute('aria-label', `Hide proxy URL for ${host}`);
    } catch (_) {
      const feedback = document.getElementById('personal-proxy-feedback');
      if (feedback) {
        feedback.textContent = 'Could not show proxy URL.';
        feedback.dataset.error = 'true';
      }
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });
})();
