// Comportamiento del panel de administración. Antes eran <script> inline
// y atributos onclick repartidos por las plantillas; viven acá para que
// la CSP del panel pueda ser script-src 'self' sin 'unsafe-inline'
// (specs/cabeceras-seguridad/). Se carga con defer: corre con el DOM ya
// parseado, así que todo se engancha una sola vez, sin depender del orden
// de las plantillas.
(function () {
  // ── Toast de éxito (patrón post/redirect/get, ?success=…) ──
  var toast = document.getElementById('toast');
  if (toast) {
    setTimeout(function () {
      toast.classList.add('toast-hide');
      setTimeout(function () { toast.remove(); }, 250);
    }, 3500);
  }

  // ── Abrir/cerrar <dialog> declarativo ──
  // <button data-open-dialog="id"> y <button data-close-dialog="id">
  // reemplazan los onclick="document.getElementById(…).showModal()".
  document.addEventListener('click', function (e) {
    var el = e.target instanceof Element ? e.target.closest('[data-open-dialog], [data-close-dialog]') : null;
    if (!el) return;
    var open = el.getAttribute('data-open-dialog');
    var close = el.getAttribute('data-close-dialog');
    var dialog = document.getElementById(open || close);
    if (!dialog) return;
    if (open) dialog.showModal(); else dialog.close();
  });

  // Un formulario de creación que volvió con error se reabre solo.
  var createModal = document.getElementById('create-user-modal');
  if (createModal && createModal.dataset.reopen) createModal.showModal();

  // ── Diálogo de confirmación compartido ──
  // Cualquier <form data-confirm="mensaje" data-confirm-title="título"
  // data-confirm-tone="danger"> pasa por este diálogo antes de enviarse.
  // Delegado en document: el diálogo está en el header, antes que los
  // formularios de la página.
  var confirmDialog = document.getElementById('confirm-modal');
  if (confirmDialog) {
    var headerEl = document.getElementById('confirm-modal-header');
    var titleEl = document.getElementById('confirm-modal-title');
    var messageEl = document.getElementById('confirm-modal-message');
    var confirmBtn = document.getElementById('confirm-modal-confirm');
    var cancelBtn = document.getElementById('confirm-modal-cancel');
    var pendingForm = null;

    document.addEventListener('submit', function (e) {
      var form = e.target;
      if (!(form instanceof HTMLFormElement) || !form.dataset.confirm) return;
      e.preventDefault();
      pendingForm = form;
      var isDanger = form.dataset.confirmTone === 'danger';
      titleEl.textContent = form.dataset.confirmTitle || 'Confirmar acción';
      messageEl.textContent = form.dataset.confirm;
      headerEl.className = 'modal-header' + (isDanger ? ' tone-danger' : '');
      confirmBtn.className = 'btn ' + (isDanger ? 'btn-danger-solid' : 'btn-primary');
      confirmDialog.showModal();
    });

    // form.submit() no vuelve a disparar 'submit': no hay bucle.
    confirmBtn.addEventListener('click', function () {
      confirmDialog.close();
      if (pendingForm) { pendingForm.submit(); pendingForm = null; }
    });
    cancelBtn.addEventListener('click', function () { confirmDialog.close(); pendingForm = null; });
    // Click en el backdrop cierra el modal.
    confirmDialog.addEventListener('click', function (e) {
      if (e.target === confirmDialog) { confirmDialog.close(); pendingForm = null; }
    });
  }

  // ── Botón de tema claro/oscuro ──
  var themeBtn = document.getElementById('theme-toggle-btn');
  if (themeBtn) {
    themeBtn.addEventListener('click', function () {
      var root = document.documentElement;
      var explicit = root.getAttribute('data-theme');
      var currentlyDark = explicit ? explicit === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches;
      var next = currentlyDark ? 'light' : 'dark';
      root.setAttribute('data-theme', next);
      localStorage.setItem('logday_admin_theme', next);
    });
  }

  // ── Resetear contraseña (users.html) ──
  // Cada fila tiene <button data-reset-user-id data-reset-email>; los
  // valores van en atributos (escapados por html/template) en vez de
  // armarse dentro de un onclick.
  var resetModal = document.getElementById('reset-password-modal');
  if (resetModal) {
    var resetForm = document.getElementById('reset-password-form');
    var resetTarget = document.getElementById('reset-password-target');
    var resetInput = document.getElementById('reset-password-input');
    var generateBtn = document.getElementById('reset-password-generate-btn');
    var copyBtn = document.getElementById('reset-password-copy-btn');

    document.addEventListener('click', function (e) {
      var row = e.target instanceof Element ? e.target.closest('[data-reset-user-id]') : null;
      if (!row) return;
      resetForm.action = '/admin/panel/users/' + encodeURIComponent(row.getAttribute('data-reset-user-id')) + '/reset-password';
      resetTarget.textContent = 'Para ' + row.getAttribute('data-reset-email') + '.';
      resetInput.value = '';
      resetInput.type = 'password';
      copyBtn.hidden = true;
      resetModal.showModal();
    });

    // Alfabeto sin caracteres ambiguos (0/O, 1/l/I) para que la
    // contraseña generada sea fácil de transcribir a mano si hace falta.
    var generatePassword = function (length) {
      var chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%^&*-_=+';
      var bytes = new Uint8Array(length);
      crypto.getRandomValues(bytes);
      var out = '';
      for (var i = 0; i < length; i++) out += chars[bytes[i] % chars.length];
      return out;
    };

    generateBtn.addEventListener('click', function () {
      resetInput.value = generatePassword(16);
      resetInput.type = 'text';
      copyBtn.hidden = false;
    });
    copyBtn.addEventListener('click', function () {
      navigator.clipboard.writeText(resetInput.value);
    });
  }

  // ── Secreto generado (settings.html) ──
  var secretInput = document.getElementById('generated-secret');
  var copySecretBtn = document.getElementById('copy-secret-btn');
  if (secretInput) secretInput.addEventListener('click', function () { secretInput.select(); });
  if (secretInput && copySecretBtn) {
    copySecretBtn.addEventListener('click', function () {
      navigator.clipboard.writeText(secretInput.value);
    });
  }
})();
