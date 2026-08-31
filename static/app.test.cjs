const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const { test } = require("node:test");

class Element {
  constructor(dataset = {}) {
    this.dataset = dataset;
    this.listeners = {};
    this.attributes = {};
    this.hidden = true;
    this.textContent = "";
  }
  addEventListener(name, listener) { (this.listeners[name] ??= []).push(listener); }
  emit(name, event = {}) { for (const listener of this.listeners[name] ?? []) listener(event); }
  setAttribute(name, value) { this.attributes[name] = value; }
  focus() { this.focused = true; this.emit("focus"); }
}

function fixture(count = 3) {
  const points = Array.from({ length: count }, (_, i) => {
    const point = new Element({ chartX: String(i * 50), chartLabel: `Date ${i}`, chartValue: `${i * 12.5} minutes` });
    point.tabIndex = i === count - 1 ? 0 : -1;
    point.setAttribute("aria-pressed", String(i === count - 1));
    return point;
  });
  const plot = new Element();
  plot.getBoundingClientRect = () => ({ left: 10, width: 200 });
  const label = new Element(), value = new Element(), hint = new Element();
  const chart = new Element();
  chart.matches = (selector) => selector === "[data-chart]";
  chart.querySelectorAll = (selector) => selector === "[data-chart-point]" ? points : [];
  chart.querySelector = (selector) => ({ "[data-chart-plot]": plot, "[data-chart-hint]": hint, "[data-chart-readout-label]": label, "[data-chart-readout-value]": value })[selector];
  const document = new Element();
  document.querySelectorAll = (selector) => selector === "[data-chart]" ? [chart] : [];
  const context = vm.createContext({ document });
  vm.runInContext(fs.readFileSync(__dirname + "/app.js", "utf8"), context);
  document.emit("DOMContentLoaded");
  return { context, chart, document, points, plot, label, value, hint };
}

function press(point, key) {
  let prevented = false;
  point.emit("keydown", { key, preventDefault() { prevented = true; } });
  return prevented;
}

test("keyboard selection updates the readout, focus, and only one tab stop", () => {
  const f = fixture();
  assert.equal(f.hint.hidden, false);
  assert.equal(press(f.points[2], "Home"), true);
  assert.equal(f.value.textContent, "0 minutes");
  assert.equal(f.points[0].focused, true);
  press(f.points[0], "ArrowRight");
  assert.equal(f.value.textContent, "12.5 minutes");
  assert.equal(f.label.textContent, "Date 1");
  assert.equal(f.points.filter(p => p.tabIndex === 0).length, 1);
  press(f.points[1], "ArrowLeft");
  assert.equal(f.points[0].attributes["aria-pressed"], "true");
  press(f.points[0], "End");
  assert.equal(f.value.textContent, "25 minutes");
  assert.equal(press(f.points[2], "Tab"), false);
});

test("keyboard stays within the dataset, including a single point", () => {
  const f = fixture(1);
  press(f.points[0], "ArrowLeft");
  press(f.points[0], "ArrowRight");
  assert.equal(f.points[0].tabIndex, 0);
  assert.equal(f.points[0].attributes["aria-pressed"], "true");
});

test("hover inspects the nearest date and touch scrolling does not change selection", () => {
  const f = fixture();
  f.plot.emit("pointermove", { pointerType: "mouse", clientX: 111 });
  assert.equal(f.value.textContent, "12.5 minutes");
  f.plot.emit("pointermove", { pointerType: "touch", clientX: 10 });
  assert.equal(f.value.textContent, "12.5 minutes");
  f.plot.emit("click", { detail: 1, clientX: 10 });
  assert.equal(f.value.textContent, "0 minutes");
});

test("pointer clicks resolve overlapping targets; keyboard clicks retain the point", () => {
  const f = fixture();
  f.points[2].emit("click");
  f.plot.emit("click", { detail: 1, clientX: 11 });
  assert.equal(f.value.textContent, "0 minutes");
  f.points[1].emit("click");
  f.plot.emit("click", { detail: 0, clientX: 0 });
  assert.equal(f.value.textContent, "12.5 minutes");
});

test("initialization is idempotent after HTMX swaps, including a chart root", () => {
  const f = fixture();
  f.document.emit("htmx:afterSwap", { target: f.chart });
  assert.equal(f.points[0].listeners.keydown.length, 1);
  assert.equal(f.plot.listeners.click.length, 1);
  delete f.chart.dataset.chartReady;
  f.document.emit("htmx:afterSwap", { target: f.chart });
  assert.equal(f.chart.dataset.chartReady, "true");
});

test("empty charts need no controls or readout", () => {
  const f = fixture(0);
  assert.equal(f.chart.dataset.chartReady, undefined);
  assert.equal(f.hint.hidden, true);
});
