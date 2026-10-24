// Aplica el tema guardado antes del primer pintado, para que no haya un
// destello del tema equivocado al cargar. Se carga en el <head> sin
// defer/async (bloqueante a propósito). Sin elección guardada, decide el
// CSS por prefers-color-scheme. Antes era un <script> inline; está en un
// archivo para que la CSP del panel no necesite 'unsafe-inline'.
(function () {
  var saved = localStorage.getItem('logday_admin_theme');
  if (saved === 'light' || saved === 'dark') {
    document.documentElement.setAttribute('data-theme', saved);
  }
})();
