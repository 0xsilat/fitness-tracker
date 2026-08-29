function initializeFormatForms(root = document) {
  root.querySelectorAll("[data-format-form]").forEach((form) => {
    const select = form.querySelector("[data-format-select]");
    if (!select) return;

    const update = () => {
      form.querySelectorAll("[data-format-fields]").forEach((fields) => {
        fields.hidden = fields.dataset.formatFields !== select.value;
      });
    };

    select.addEventListener("change", update);
    update();
  });
}

function scrollCalendarsToNewest(root = document) {
  root.querySelectorAll("[data-scroll-end]").forEach((calendar) => {
    calendar.scrollLeft = calendar.scrollWidth;
  });
}

document.addEventListener("DOMContentLoaded", () => {
  initializeFormatForms();
  scrollCalendarsToNewest();

  document.querySelectorAll('a[href^="#"]').forEach((link) => {
    link.addEventListener("click", () => {
      const target = document.querySelector(link.getAttribute("href"));
      if (target instanceof HTMLDetailsElement) target.open = true;
    });
  });
});

document.addEventListener("htmx:afterSwap", (event) => {
  initializeFormatForms(event.target);
  scrollCalendarsToNewest(event.target);
});
