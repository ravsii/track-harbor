(() => {
  document.addEventListener('submit', async event => {
    const form = event.target;
    if (form.id !== 'youtube-add-form') return;
    event.preventDefault();
    const button = form.querySelector('button[type="submit"]');
    const field = form.elements.video;
    const feedback = document.getElementById('youtube-feedback');
    button.disabled = true;
    feedback.textContent = 'Adding video…';
    feedback.dataset.error = 'false';
    feedback.setAttribute('role', 'status');
    field.removeAttribute('aria-invalid');
    try {
      const response = await fetch(form.action, {
        method: 'POST',
        headers: {'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json'},
        body: new URLSearchParams(new FormData(form))
      });
      if (!response.ok) {
        feedback.textContent = (await response.text()).trim() || 'Could not add this video. Try again.';
        feedback.dataset.error = 'true';
        feedback.setAttribute('role', 'alert');
        if (response.status === 400) field.setAttribute('aria-invalid', 'true');
        return;
      }
      const result = await response.json();
      if (!form.isConnected) return;
      await htmx.ajax('GET', '/?source=youtube', {target: '#content', select: '#content', swap: 'outerHTML'});
      if (!result.added) {
        const message = document.getElementById('youtube-feedback');
        if (message) message.textContent = 'This video is already in your collection.';
      }
    } catch (_) {
      if (feedback.isConnected) {
        feedback.textContent = 'Could not add this video. Check your connection and try again.';
        feedback.dataset.error = 'true';
        feedback.setAttribute('role', 'alert');
      }
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });
})();
