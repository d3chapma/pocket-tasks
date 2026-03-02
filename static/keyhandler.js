function handleKeydown(event, state) {
  if (state.showForm) {
    if (event.key === "Escape") {
      event.preventDefault();
      return { ...state, showForm: false };
    }
    return state;
  }

  let { selectedIndex, hideCompleted } = state;
  const maxIndex = state.maxIndex;

  switch (event.key) {
    case "j":
      event.preventDefault();
      selectedIndex = Math.min(selectedIndex + 1, maxIndex);
      break;

    case "k":
      event.preventDefault();
      selectedIndex = Math.max(selectedIndex - 1, 0);
      break;

    case " ":
      event.preventDefault();
      clickSelected(selectedIndex, "[data-toggle]");
      break;

    case "y":
      if (window._pendingY) {
        clearTimeout(window._pendingY);
        window._pendingY = null;
        copySelected(selectedIndex);
      } else {
        window._pendingY = setTimeout(function () {
          window._pendingY = null;
        }, 500);
      }
      break;

    case "g":
      if (window._pendingG) {
        clearTimeout(window._pendingG);
        window._pendingG = null;
        selectedIndex = 0;
      } else {
        window._pendingG = setTimeout(function () {
          window._pendingG = null;
        }, 500);
      }
      break;

    case "G":
      event.preventDefault();
      selectedIndex = maxIndex;
      break;

    case "i":
      event.preventDefault();
      setTimeout(function () {
        document.getElementById("new-task").focus();
      }, 0);
      return { ...state, showForm: true };

    case "x":
      hideCompleted = !hideCompleted;
      break;

    case "d":
      if (window._pendingD) {
        clearTimeout(window._pendingD);
        window._pendingD = null;
        clickSelected(selectedIndex, "[data-delete]");
      } else {
        window._pendingD = setTimeout(function () {
          window._pendingD = null;
        }, 500);
      }
      break;

    case "K":
      if (selectedIndex < state.activeLen && selectedIndex > 0) {
        event.preventDefault();
        clickSelected(selectedIndex, "[data-move-up]");
      }
      break;

    case "J":
      if (selectedIndex < state.activeLen - 1) {
        event.preventDefault();
        clickSelected(selectedIndex, "[data-move-down]");
      }
      break;

    case "H":
      if (selectedIndex < state.activeLen && selectedIndex > 0) {
        event.preventDefault();
        clickSelected(selectedIndex, "[data-move-top]");
      }
      break;

    case "L":
      if (
        selectedIndex < state.activeLen &&
        selectedIndex < state.activeLen - 1
      ) {
        event.preventDefault();
        clickSelected(selectedIndex, "[data-move-bottom]");
      }
      break;
  }

  return { ...state, selectedIndex, hideCompleted };
}

function clickSelected(selectedIndex, selector) {
  const items = document.querySelectorAll(".task-slider li[data-task]");
  const el = items[selectedIndex];
  if (el) {
    const btn = el.querySelector(selector);
    if (btn) btn.click();
  }
}

function copySelected(selectedIndex) {
  const items = document.querySelectorAll(".task-slider li[data-task]");
  const el = items[selectedIndex];
  if (el) {
    const title = el.querySelector(".task-title");
    if (title) {
      navigator.clipboard.writeText(title.textContent);
      showToast("Yanked.");
    }
  }
}

function showToast(message) {
  const toast = document.getElementById("toast");
  toast.textContent = message;
  toast.classList.add("visible");

  if (window._toastTimeout) {
    clearTimeout(window._toastTimeout);
  }

  window._toastTimeout = setTimeout(function () {
    toast.classList.remove("visible");
  }, 2000);
}
