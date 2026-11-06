// runlike site: theme toggle, copy buttons, platform detection. The page works without it.
(function () {
  var d = document, h = d.documentElement, KEY = 'runlike-theme', saved;
  try { saved = localStorage.getItem(KEY); } catch (e) {}
  if (saved === 'light' || saved === 'dark') h.dataset.theme = saved; // before first paint: this script is in <head>
  h.classList.add('js'); // the hero terminals wait for the reader (CSS pauses them until .play)

  function isDark() {
    return h.dataset.theme ? h.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  }

  d.addEventListener('DOMContentLoaded', function () {
    var t = d.querySelector('.theme');
    function label() { t.setAttribute('aria-label', 'Switch to ' + (isDark() ? 'light' : 'dark') + ' theme'); }
    t.hidden = false;
    label();
    t.addEventListener('click', function () {
      var next = isDark() ? 'light' : 'dark';
      h.dataset.theme = next;
      try { localStorage.setItem(KEY, next); } catch (e) {}
      label();
    });

    // The story: the step being read picks the terminal (wide screens); each terminal types when it shows.
    var steps = [].slice.call(d.querySelectorAll('.step')), terms = [].slice.call(d.querySelectorAll('.term'));
    var jumps = [].slice.call(d.querySelectorAll('.jump a'));
    var sticky = { matches: false }; // steps are stacked: each terminal types when it scrolls into view
    function activate(i) {
      terms.forEach(function (t, j) { t.classList.toggle('active', j === i); if (j === i) t.classList.add('play'); });
      jumps.forEach(function (a, j) { if (j === i) a.setAttribute('aria-current', 'step'); else a.removeAttribute('aria-current'); });
    }
    if ('IntersectionObserver' in window) {
      var byStep = new IntersectionObserver(function (es) {
        es.forEach(function (e) { if (e.isIntersecting && sticky.matches) activate(steps.indexOf(e.target)); });
      }, { rootMargin: '-50% 0px -50% 0px' }); // the step crossing the middle of the window
      steps.forEach(function (s) { byStep.observe(s); });
      var seen = new IntersectionObserver(function (es) {
        es.forEach(function (e) { if (e.isIntersecting && !sticky.matches) e.target.classList.add('play'); });
      }, { threshold: .2 });
      terms.forEach(function (t) { seen.observe(t); });
    } else terms.forEach(function (t) { t.classList.add('play'); });
    if (sticky.matches) activate(0);

    d.querySelectorAll('.copy').forEach(function (b) {
      if (!navigator.clipboard) return;
      b.hidden = false;
      b.addEventListener('click', function () {
        navigator.clipboard.writeText(b.parentNode.querySelector('code').textContent).then(function () {
          b.textContent = 'Copied';
          b.classList.add('done');
          setTimeout(function () { b.textContent = 'Copy'; b.classList.remove('done'); }, 1500);
        });
      });
    });

    // Best effort: feature the download for this machine. Apple Silicon is the only macOS build.
    var p = navigator.platform || '', ua = navigator.userAgent;
    var os = /Mac/i.test(p) ? 'mac' : /Win/i.test(p) ? 'windows' : /Linux|X11/i.test(p) ? 'linux' : '';
    var arch = /aarch64|arm64|armv8/i.test(p + ' ' + ua) ? 'arm64' : 'amd64';
    function feature() {
      if (os === 'mac') arch = 'arm64';
      var a = os && d.querySelector('.dl[data-os=' + os + '][data-arch=' + arch + ']');
      if (!a) return;
      a.classList.add('featured');
      var det = d.querySelector('.detect');
      det.querySelector('b').textContent = a.querySelector('.os').textContent + ', ' + a.querySelector('.arch').textContent;
      det.hidden = false;
      if (os !== 'windows') d.querySelector('.asset').textContent = a.querySelector('code').textContent;
    }
    var uad = navigator.userAgentData;
    if (uad && uad.getHighEntropyValues && os !== 'mac') {
      uad.getHighEntropyValues(['architecture']).then(function (v) {
        if (v.architecture === 'arm') arch = 'arm64'; else if (v.architecture === 'x86') arch = 'amd64';
      }).then(feature, feature);
    } else feature();
  });
})();
