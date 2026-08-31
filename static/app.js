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

function initializeCharts(root = document) {
  const charts = [...root.querySelectorAll("[data-chart]")];
  if (root.matches?.("[data-chart]")) charts.unshift(root);
  charts.forEach((chart) => {
    if (chart.dataset.chartReady) return;
    const points = [...chart.querySelectorAll("[data-chart-point]")];
    if (!points.length) return;
    chart.dataset.chartReady = "true";
    chart.querySelector("[data-chart-hint]").hidden = false;
    const plot = chart.querySelector("[data-chart-plot]");
    const label = chart.querySelector("[data-chart-readout-label]");
    const value = chart.querySelector("[data-chart-readout-value]");
    let selected = points.length - 1;

    const select = (index, focus = false) => {
      index = Math.max(0, Math.min(points.length - 1, index));
      if (index !== selected) {
        points[selected].tabIndex = -1;
        points[selected].setAttribute("aria-pressed", "false");
        selected = index;
        points[selected].tabIndex = 0;
        points[selected].setAttribute("aria-pressed", "true");
        label.textContent = points[selected].dataset.chartLabel;
        value.textContent = points[selected].dataset.chartValue;
      }
      if (focus) points[selected].focus({ preventScroll: true });
    };

    points.forEach((point, index) => {
      point.addEventListener("focus", () => select(index));
      point.addEventListener("click", () => select(index));
      point.addEventListener("keydown", (event) => {
        let next;
        if (event.key === "ArrowLeft") next = index - 1;
        if (event.key === "ArrowRight") next = index + 1;
        if (event.key === "Home") next = 0;
        if (event.key === "End") next = points.length - 1;
        if (next === undefined) return;
        event.preventDefault();
        select(next, true);
      });
    });

    // The whole plot is a hit target, so dense series are usable on touch screens.
    const inspect = (event) => {
      const bounds = plot.getBoundingClientRect();
      if (!bounds.width) return;
      const x = (event.clientX - bounds.left) / bounds.width * 100;
      let nearest = 0;
      points.forEach((point, index) => {
        if (Math.abs(Number(point.dataset.chartX) - x) < Math.abs(Number(points[nearest].dataset.chartX) - x)) nearest = index;
      });
      select(nearest);
    };
    plot.addEventListener("pointermove", (event) => {
      if (event.pointerType !== "touch") inspect(event);
    });
    plot.addEventListener("click", (event) => {
      // Pointer clicks choose the nearest date even when hit targets overlap.
      // Keyboard-generated clicks (detail=0) retain their focused point.
      if (event.detail > 0) inspect(event);
    });
  });
}

document.addEventListener("DOMContentLoaded", () => {
  initializeFormatForms();
  scrollCalendarsToNewest();
  initializeCharts();

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
  initializeCharts(event.target);
});
