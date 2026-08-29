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

// UI state only: the ordinary per-minute inputs remain the submitted data.
const emomViewState = new Map();

function emomRows(editor) {
  return Array.from(editor.querySelectorAll('.set-row:not(.header)')).map((row) => ({
    row,
    reps: row.querySelector('input[name^="reps_"]'),
    weight: row.querySelector('input[name^="weight_"]'),
    skip: row.querySelector('input[name^="skipped_"]'),
  }));
}

function initializeEMOMEditors(root = document) {
  root.querySelectorAll('[data-emom-editor]').forEach((editor) => {
    if (editor.dataset.initialized) return;
    editor.dataset.initialized = 'true';
    const id = editor.closest('.movement').id;
    const quick = editor.querySelector('[data-emom-quick]');
    const reps = editor.querySelector('[data-emom-reps]');
    const weight = editor.querySelector('[data-emom-weight]');
    const details = editor.querySelector('[data-emom-minutes]');
    const summary = editor.querySelector('[data-emom-summary]');
    const feedback = editor.querySelector('[data-emom-feedback]');
    const apply = editor.querySelector('[data-emom-apply]');
    const confirmation = editor.querySelector('[data-emom-confirm]');
    const replace = editor.querySelector('[data-emom-replace]');
    const cancel = editor.querySelector('[data-emom-cancel]');
    const rows = emomRows(editor);
    const previous = emomViewState.get(id);

    const updateSummary = () => {
      const active = rows.filter((set) => !set.skip.checked);
      const completed = active.filter((set) => Number(set.reps.value) > 0);
      const skipped = rows.length - active.length;
      const incomplete = active.length - completed.length;
      const repValues = new Set(completed.map((set) => Number(set.reps.value)));
      const weightValues = new Set(completed.map((set) => Number(set.weight?.value || 0)));
      const parts = [`${completed.length} of ${rows.length} minutes logged`];
      if (completed.length) {
        parts.push(repValues.size === 1 ? `${completed[0].reps.value} reps each` : 'Reps vary');
        if (weight) parts.push(weightValues.size === 1 ? `${Number(completed[0].weight.value)} kg each` : 'Weights vary');
      }
      if (incomplete) parts.push(`${incomplete} incomplete`);
      if (skipped) parts.push(`${skipped} skipped`);
      summary.textContent = parts.join(' · ');
      apply.disabled = active.length === 0;
      rows.forEach((set) => set.row.classList.toggle('skipped', set.skip.checked));
    };

    const repValues = new Set(rows.map((set) => Number(set.reps.value)));
    const weightValues = new Set(rows.map((set) => Number(set.weight?.value || 0)));
    const hasExceptions = repValues.size > 1 || weightValues.size > 1 || rows.some((set) => set.skip.checked);
    details.open = previous ? previous.open || rows.some((set) => !previous.ids.includes(set.reps.name)) : hasExceptions;
    if (previous) {
      reps.value = previous.reps;
      if (weight) weight.value = previous.weight;
    } else if (repValues.size === 1 && Number(rows[0]?.reps.value) > 0) {
      reps.value = rows[0].reps.value;
    }
    quick.hidden = false;
    updateSummary();

    // Scratch fields have no form owner or names: they must neither submit nor
    // block saving individual rows, even when an unfinished shortcut is invalid.
    quick.addEventListener('change', (event) => event.stopPropagation());
    quick.addEventListener('input', () => {
      confirmation.hidden = true;
      feedback.textContent = 'Press Apply to update the minutes.';
    });
    quick.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && event.target.matches('input')) {
        event.preventDefault();
        apply.click();
      }
    });
    details.addEventListener('input', updateSummary);
    details.addEventListener('change', () => {
      updateSummary();
      feedback.textContent = '';
      confirmation.hidden = true;
    });
    // Reveal an invalid individual input before native form validation focuses it.
    details.addEventListener('invalid', () => { details.open = true; }, true);

    const applyValues = (confirmed = false) => {
      if (!reps.reportValidity() || (weight && !weight.reportValidity())) return;
      const active = rows.filter((set) => !set.skip.checked);
      if (!active.length) return;
      const sharedWeight = weight && weight.value !== '' ? weight.value : null;
      const replacing = active.some((set) =>
        (Number(set.reps.value) > 0 && Number(set.reps.value) !== Number(reps.value)) ||
        (sharedWeight !== null && Number(set.weight.value) !== Number(sharedWeight))
      );
      if (replacing && !confirmed) {
        confirmation.hidden = false;
        feedback.textContent = 'Confirm replacement of existing values, or cancel to keep them.';
        replace.focus();
        return;
      }
      confirmation.hidden = true;
      active.forEach((set) => {
        set.reps.value = reps.value;
        if (sharedWeight !== null) set.weight.value = sharedWeight;
      });
      updateSummary();
      // One bubbling event schedules the existing form autosave after all rows
      // have changed. Immediate Save/Complete also sees those values.
      active[0].reps.dispatchEvent(new Event('change', { bubbles: true }));
      feedback.textContent = `Applied to ${active.length} ${active.length === 1 ? 'minute' : 'minutes'}.`;
      if (confirmed) apply.focus();
    };
    apply.addEventListener('click', () => applyValues());
    replace.addEventListener('click', () => applyValues(true));
    cancel.addEventListener('click', () => {
      confirmation.hidden = true;
      feedback.textContent = 'Existing minute values kept.';
      apply.focus();
    });
  });
}

document.addEventListener('htmx:beforeSwap', () => {
  document.querySelectorAll('[data-emom-editor]').forEach((editor) => {
    emomViewState.set(editor.closest('.movement').id, {
      open: editor.querySelector('[data-emom-minutes]').open,
      ids: emomRows(editor).map((set) => set.reps.name),
      reps: editor.querySelector('[data-emom-reps]').value,
      weight: editor.querySelector('[data-emom-weight]')?.value || '',
    });
  });
});

document.addEventListener("DOMContentLoaded", () => {
  initializeFormatForms();
  scrollCalendarsToNewest();
  initializeEMOMEditors();

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
  initializeEMOMEditors(event.target);
});
