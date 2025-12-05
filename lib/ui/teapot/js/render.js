window.teapot = { styles: new Set(), dependencies: new Set() }

// populate requried headers when sending HTMX requests
document.addEventListener('DOMContentLoaded', () => {
  document.body.addEventListener('htmx:configRequest', (e) => {
    e.detail.headers['X-Teapot-Styles'] = JSON.stringify(Array.from(window.teapot.styles));
    e.detail.headers['X-Teapot-Dependencies'] = JSON.stringify(Array.from(window.teapot.dependencies));
  });
});
