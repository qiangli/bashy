/* bashy desk UI kit — shared page behaviour for every desk tool.
   Copy buttons flip to a "Copied" state; badges render from status;
   theme toggle overrides prefers-color-scheme. No dependencies. */
(function () {
  'use strict';

  function flashCopied(btn) {
    var prev = btn.textContent;
    btn.setAttribute('data-copied', 'true');
    btn.textContent = 'Copied';
    setTimeout(function () {
      btn.setAttribute('data-copied', 'false');
      btn.textContent = prev;
    }, 1600);
  }

  async function copyText(btn) {
    var text = btn.getAttribute('data-copy') || '';
    try {
      await navigator.clipboard.writeText(text);
      flashCopied(btn);
    } catch (e) {
      var msg = document.querySelector('[data-desk-alert]');
      if (msg) msg.textContent = 'Copy this instead: ' + text;
    }
  }

  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-desk-copy]');
    if (btn) copyText(btn);
    var theme = ev.target.closest('[data-desk-theme]');
    if (theme) {
      var root = document.documentElement;
      root.setAttribute('data-theme', root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark');
    }
  });

  // Badge helper for tools that render status client-side.
  window.deskBadge = function (status, label) {
    var s = document.createElement('span');
    s.className = 'desk-badge';
    s.setAttribute('data-status', status);
    s.textContent = label || status;
    return s;
  };
})();
