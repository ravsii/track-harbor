(() => {
  let expired = false;
  const expire = () => {
    if (expired) return;
    expired = true;
    document.dispatchEvent(new Event('auth:expired'));
    window.location.assign('/login');
  };
  const originalFetch = window.fetch.bind(window);
  window.fetch = async (input, options = {}) => {
    const url = new URL(input instanceof Request ? input.url : input, location.href);
    if (url.origin === location.origin) {
      const headers = new Headers(options.headers ?? (input instanceof Request ? input.headers : undefined));
      headers.set('X-TrackHarbor-User', document.body.dataset.userId);
      options = {...options, headers};
    }
    const response = await originalFetch(input, options);
    if (response.status === 401 || (response.status === 409 && response.headers.get('HX-Redirect') === '/login')) expire();
    return response;
  };
  const status = async () => {
    try {
      const response = await fetch('/auth/status', {cache: 'no-store'});
      if (!response.ok) return;
      const account = await response.json();
      if (String(account.id) !== document.body.dataset.userId) { expire(); return; }
      const warning = document.getElementById('bootstrap-warning');
      if (warning) warning.hidden = !account.bootstrap;
    } catch (_) { /* Network outages do not invalidate an otherwise valid session. */ }
  };
  document.addEventListener('htmx:afterSwap', status);
  document.addEventListener('htmx:responseError', event => {
    if (event.detail.xhr.status === 401) { expire(); return; }
    const form = event.detail.elt;
    if (!form?.closest('.users-panel')) return;
    const feedback = document.getElementById('users-feedback');
    if (feedback) feedback.textContent = event.detail.xhr.responseText || 'Could not update user.';
  });
  document.addEventListener('submit', event => {
    if (event.target.id === 'logout-form') document.dispatchEvent(new Event('auth:expired'));
  });
  const menu = document.getElementById('account-menu');
  if (menu) {
    document.addEventListener('click', event => {
      if (!menu.contains(event.target)) menu.open = false;
    });
    document.addEventListener('keydown', event => {
      if (event.key === 'Escape' && menu.open) {
        menu.open = false;
        menu.querySelector('summary').focus();
      }
    });
    menu.addEventListener('focusout', () => {
      requestAnimationFrame(() => {
        if (!menu.contains(document.activeElement)) menu.open = false;
      });
    });
  }
  setInterval(status, 30000);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) status(); });
})();
