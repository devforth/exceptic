// Small enhancements on top of the server-rendered pages. The UI works without them.

let copyToastTimeout;

// Copy buttons (the "copy-button" partial): copy the text of their target.
document.addEventListener("click", async (event) => {
  const button = event.target.closest("button[data-copy]");
  if (!button) return;

  // The text is in the element it names: a textarea's value or an element's text.
  const source = document.getElementById(button.dataset.copy);
  const status = document.getElementById("copy-status");

  if (!source) return;

  const isField = source instanceof HTMLTextAreaElement || source instanceof HTMLInputElement;
  const text = isField ? source.value : source.textContent;
  const done = button.dataset.copied || "Copied to clipboard.";

  clearTimeout(copyToastTimeout);
  if (status) status.textContent = "";
  button.dataset.title ??= button.title;
  button.disabled = true;
  try {
    if (!navigator.clipboard?.writeText) throw new Error("Clipboard unavailable");
    await navigator.clipboard.writeText(text);
    button.title = "Copied!";
    if (status) status.textContent = done;
  } catch {
    if (source instanceof HTMLInputElement && ["password", "hidden"].includes(source.type)) {
      // Password fields cannot be copied with execCommand in some browsers.
      // Keep the fallback off screen and remove it before returning focus.
      const previousFocus = document.activeElement;
      const fallback = document.createElement("textarea");
      fallback.value = text;
      fallback.readOnly = true;
      fallback.className = "secret-copy-fallback";
      fallback.setAttribute("aria-hidden", "true");
      fallback.tabIndex = -1;
      document.body.append(fallback);
      let copied = false;
      try {
        fallback.focus({ preventScroll: true });
        fallback.select();
        copied = document.execCommand("copy");
      } catch {
        // Leave the token masked even when clipboard access is unavailable.
      } finally {
        fallback.remove();
        previousFocus?.focus({ preventScroll: true });
      }
      if (status) status.textContent = copied ? done : "Could not copy. Allow clipboard access and try again.";
      if (copied) button.title = "Copied!";
      return;
    }
    // HTTP and browsers without clipboard permission can still copy a selection.
    const wasHidden = source.hidden;
    if (isField) {
      source.hidden = false;
      source.classList.add("copy-fallback");
      source.readOnly = true;
      source.focus();
      source.select();
    } else {
      const range = document.createRange();
      range.selectNodeContents(source);
      window.getSelection().removeAllRanges();
      window.getSelection().addRange(range);
    }
    let copied = false;
    try {
      copied = document.execCommand("copy");
    } catch {
      // Keep the selected text available for manual copying.
    }
    if (isField) source.hidden = copied && wasHidden;
    if (status) status.textContent = copied ? done : "Copy the selected text with Ctrl+C or ⌘C.";
    if (copied) button.title = "Copied!";
  } finally {
    button.disabled = false;
    setTimeout(() => { button.title = button.dataset.title; }, 2000);
    copyToastTimeout = setTimeout(() => { if (status) status.textContent = ""; }, 5000);
  }
});

// Show exact times in the browser's time zone (the server renders UTC).
function localizeTimes(root) {
  root.querySelectorAll("time[datetime]").forEach((element) => {
    const date = new Date(element.getAttribute("datetime"));

    if (!Number.isNaN(date.getTime())) {
      element.title = date.toLocaleString();
    }
  });
}

// Connection form: show only the fields of the selected type, lock the rest
// until an email address is confirmed, and hide the project list while "All
// projects" is checked (it covers every project).
function updateConnectionForm(form) {
  // Only a new connection has type radios; an existing one shows its own fields.
  const selected = form.querySelector('input[name="type"]:checked');

  if (selected) {
    form.querySelectorAll(".type-fields").forEach((fields) => {
      fields.hidden = fields.dataset.type !== selected.value;
      // The fields of the other types are neither checked nor sent: a hidden
      // invalid field (a browser filling a login into it) would block the form.
      fields.querySelectorAll("input, select, textarea").forEach((field) => {
        field.disabled = fields.hidden;
      });
    });
  }

  // The name placeholder shows what an empty name becomes: the start of the
  // address, or the type's own.
  const name = form.querySelector('input[name="name"]');

  if (name && selected) {
    const address = form.querySelector(`.type-fields[data-type="${selected.value}"] input[name="address"]`);
    const local = address ? address.value.split("@")[0].trim() : "";

    name.placeholder = local || selected.dataset.namePlaceholder || "";
  }

  // A chat's test button works once the fields it needs are filled.
  form.querySelectorAll("button[data-test-needs]").forEach((button) => {
    const fields = button.closest(".type-fields") || form;
    const filled = (name) => {
      const input = fields.querySelector(`[name="${name}"]`);
      return Boolean(input && (input.value.trim() !== "" || input.dataset.saved !== undefined));
    };

    button.disabled = !button.dataset.testNeeds.split("|").some((group) => group.split(" ").every(filled));
  });

  // "Find chat ID" opens the bot's getUpdates once the token is valid (its
  // pattern leaves nothing to escape in the URL). A saved token is not in the
  // page, so it needs to be typed again.
  const botToken = form.querySelector('input[name="bot_token"]');

  form.querySelectorAll("a[data-telegram-updates]").forEach((link) => {
    const token = botToken && !botToken.disabled ? botToken.value.trim() : "";
    const valid = token !== "" && botToken.checkValidity();

    if (valid) {
      link.href = `https://api.telegram.org/bot${token}/getUpdates`;
    } else {
      link.removeAttribute("href");
    }

    link.classList.toggle("disabled", !valid);
    link.setAttribute("aria-disabled", String(!valid));
  });

  // An email connection is filled in only after its address is confirmed.
  const rest = form.querySelector(".connection-rest");
  const emailStep = form.querySelector(".type-fields:not([hidden]) .email-step");

  if (rest) {
    rest.disabled = Boolean(emailStep) && !emailStep.hasAttribute("data-verified");
  }

  const allProjects = form.querySelector("input.all-projects");
  const projectChoices = form.querySelector(".project-choices");

  if (allProjects && projectChoices) {
    projectChoices.hidden = allProjects.checked;
  }
}

// Closed form sections (details.form-section) show in their heading what is
// set inside; it follows the fields as they change. [data-summary-of] lists
// the badges of the checked boxes of that name (data-summary-all-text while
// its data-summary-all box is checked).
function updateSectionSummaries(form) {
  form.querySelectorAll("[data-summary-of]").forEach((target) => {
    const all = target.dataset.summaryAll && form.querySelector(`input[name="${target.dataset.summaryAll}"]`);

    if (all && all.checked) {
      target.textContent = target.dataset.summaryAllText;
      return;
    }

    const badges = [...form.querySelectorAll(`input[name="${target.dataset.summaryOf}"]:checked`)].map(summaryBadge);

    if (badges.length === 0) {
      const none = document.createElement("span");
      none.className = "muted";
      none.textContent = "none";
      badges.push(none);
    }

    target.replaceChildren(...badges);
  });
}

// The badge a checkbox shows next to it, or a project badge of its name.
function summaryBadge(input) {
  const label = input.closest("label");
  const badge = label && label.querySelector(".badge");

  if (badge) {
    return badge.cloneNode(true);
  }

  const name = label ? label.textContent.trim() : input.value;
  const project = document.createElement("span");
  project.className = "project";
  project.dataset.project = name;
  project.textContent = name;

  return project;
}

["change", "input"].forEach((type) => {
  document.addEventListener(type, (event) => {
    const form = event.target.form;

    if (form && form.querySelector(".form-section")) {
      updateSectionSummaries(form);
    }
  });
});

// Collapse or expand the menu; the cookie lets the server render the next
// page the same way, so it does not jump on load.
document.addEventListener("click", (event) => {
  const toggle = event.target.closest(".sidebar-toggle");

  if (!toggle) {
    return;
  }

  const collapsed = toggle.closest(".app").classList.toggle("sidebar-collapsed");

  toggle.setAttribute("aria-expanded", String(!collapsed));
  document.cookie = `${toggle.dataset.cookie}=${collapsed ? toggle.dataset.collapsed : "expanded"}; path=/; max-age=31536000; SameSite=Lax`;
});

// Esc goes back like the page's "← …" link (data-back), unless a dialog or
// popover is open (they close themselves). Not while typing: there Esc
// belongs to the field.
//
// The page changes on keyup, and keydown cancels the browser's own Esc:
// Firefox stops loading the page on Esc, which would cancel a navigation
// started on keydown, so the page would just stay.
function backLink(event) {
  const busy = document.querySelector("dialog[open], [popover].open");
  const typing = event.target instanceof Element && event.target.closest("input, textarea, select, [contenteditable]");

  return busy || typing ? null : document.querySelector("a[data-back]");
}

// The link to follow on the coming keyup, decided on keydown: an Esc that
// closes a dialog must not also leave the page once the dialog is gone.
let backOnKeyup = null;

document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || event.defaultPrevented) {
    return;
  }

  backOnKeyup = backLink(event);

  if (backOnKeyup) {
    event.preventDefault();
  }
});

document.addEventListener("keyup", (event) => {
  if (event.key === "Escape" && backOnKeyup) {
    window.location.href = backOnKeyup.href;
  }

  backOnKeyup = null;
});

function enhance() {
  localizeTimes(document);
  document.querySelectorAll("form.connection-form").forEach(updateConnectionForm);
}

document.addEventListener("change", (event) => {
  const form = event.target.closest("form.connection-form");

  if (!form) {
    return;
  }

  // A new type brings its default for issue links (data-issue-link), unless
  // the box was set by hand.
  const issueLink = form.querySelector('input[name="include_issue_link"]');

  if (event.target === issueLink) {
    issueLink.dataset.touched = "true";
  } else if (event.target.matches('input[name="type"]') && issueLink && !issueLink.dataset.touched) {
    issueLink.checked = event.target.dataset.issueLink === "true";
  }

  updateConnectionForm(form);
});

// Submit the form of a field whose events do not reach it: one outside its
// form (form= attribute, like the page size under a list).
document.addEventListener("change", (event) => {
  const field = event.target;

  if (field.form && field.hasAttribute("form") && !field.form.contains(field)) {
    field.form.requestSubmit();
  }
});

document.addEventListener("input", (event) => {
  if (event.target.matches("form.connection-form input")) {
    updateConnectionForm(event.target.form);
  }
});

// Buttons with data-confirm (deletes, resets) ask before sending.
// Capture phase: the request must not start before the answer.
document.addEventListener("click", (event) => {
  const button = event.target.closest("button[data-confirm]");

  if (!button) {
    return;
  }

  if (!window.confirm(button.dataset.confirm)) {
    event.preventDefault();
    event.stopImmediatePropagation();
  }
}, true);

document.addEventListener("click", (event) => {
  // Select the whole value (a DSN) on click, ready to copy.
  if (event.target.matches("input.mono-field")) {
    event.target.select();
  }
});

document.addEventListener("DOMContentLoaded", enhance);
// After an outerHTML swap the event points at the removed element, so rescan the page.
document.addEventListener("htmx:afterSettle", enhance);

// Popovers close themselves on Esc; mark the open ones so Esc does not also go back.
document.addEventListener("toggle", (event) => {
  if (event.target instanceof Element && event.target.hasAttribute("popover")) {
    event.target.classList.toggle("open", event.newState === "open");
  }
}, true);
