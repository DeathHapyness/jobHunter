// Pequenos comportamentos da interface (sem dependências além do HTMX).
(function () {
  // Fecha dropdowns ao clicar fora; só um aberto por vez.
  document.addEventListener("click", function (e) {
    document.querySelectorAll("details.drop[open]").forEach(function (d) {
      if (!d.contains(e.target)) d.removeAttribute("open");
    });
  });
  document.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d.matches || !d.matches("details.drop") || !d.open) return;
    document.querySelectorAll("details.drop[open]").forEach(function (o) {
      if (o !== d) o.removeAttribute("open");
    });
  }, true);
  // Campo de busca dentro de listas longas (ex.: cidade). Não dispara o filtro.
  document.addEventListener("input", function (e) {
    var el = e.target;
    if (!el.matches || !el.matches("[data-filter-list]")) return;
    e.stopPropagation();
    var q = el.value.toLowerCase();
    el.parentElement.querySelectorAll("label.check").forEach(function (l) {
      l.hidden = q !== "" && l.textContent.toLowerCase().indexOf(q) < 0;
    });
  }, true);
  // Atualiza o rótulo do dropdown ("cidade: 2") ao marcar/desmarcar.
  document.addEventListener("change", function (e) {
    var d = e.target.closest && e.target.closest("details.drop");
    if (!d) return;
    var s = d.querySelector("summary");
    var base = s.getAttribute("data-label");
    var n = d.querySelectorAll("input[type=checkbox]:checked").length;
    s.textContent = base + (n ? ": " + n : "") + " ▾";
    s.classList.toggle("on", n > 0);
  });
})();
