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
        scheduleMoveAnimation(selectedIndex, "up", state.activeLen);
        clickSelected(selectedIndex, "[data-move-up]");
      }
      break;

    case "J":
      if (selectedIndex < state.activeLen - 1) {
        event.preventDefault();
        scheduleMoveAnimation(selectedIndex, "down", state.activeLen);
        clickSelected(selectedIndex, "[data-move-down]");
      }
      break;

    case "H":
      if (selectedIndex < state.activeLen && selectedIndex > 0) {
        event.preventDefault();
        scheduleMoveAnimation(selectedIndex, "top", state.activeLen);
        clickSelected(selectedIndex, "[data-move-top]");
      }
      break;

    case "L":
      if (
        selectedIndex < state.activeLen &&
        selectedIndex < state.activeLen - 1
      ) {
        event.preventDefault();
        scheduleMoveAnimation(selectedIndex, "bottom", state.activeLen);
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

var _pendingMoveAnim = null;

function scheduleMoveAnimation(selectedIndex, direction, activeLen) {
  var items = Array.from(
    document.querySelectorAll(".task-slider li[data-task]"),
  );
  var ITEM_HEIGHT = 80;
  var animItems = [];

  if (direction === "down") {
    var a = items[selectedIndex];
    var b = items[selectedIndex + 1];
    if (a && b && a.dataset.id && b.dataset.id) {
      animItems = [
        { id: a.dataset.id, delta: -ITEM_HEIGHT },
        { id: b.dataset.id, delta: ITEM_HEIGHT },
      ];
    }
  } else if (direction === "up") {
    var a = items[selectedIndex];
    var b = items[selectedIndex - 1];
    if (a && b && a.dataset.id && b.dataset.id) {
      animItems = [
        { id: a.dataset.id, delta: ITEM_HEIGHT },
        { id: b.dataset.id, delta: -ITEM_HEIGHT },
      ];
    }
  } else if (direction === "top") {
    var a = items[selectedIndex];
    if (a && a.dataset.id) {
      animItems.push({ id: a.dataset.id, delta: selectedIndex * ITEM_HEIGHT });
      for (var i = 0; i < selectedIndex; i++) {
        if (items[i] && items[i].dataset.id) {
          animItems.push({ id: items[i].dataset.id, delta: -ITEM_HEIGHT });
        }
      }
    }
  } else if (direction === "bottom") {
    var a = items[selectedIndex];
    if (a && a.dataset.id) {
      var moveCount = activeLen - 1 - selectedIndex;
      animItems.push({ id: a.dataset.id, delta: -moveCount * ITEM_HEIGHT });
      for (var i = selectedIndex + 1; i < activeLen; i++) {
        if (items[i] && items[i].dataset.id) {
          animItems.push({ id: items[i].dataset.id, delta: ITEM_HEIGHT });
        }
      }
    }
  }

  if (animItems.length > 0) {
    _pendingMoveAnim = animItems;
  }
}

function setupMoveAnimations() {
  var observer = new MutationObserver(function () {
    if (!_pendingMoveAnim) return;
    var animItems = _pendingMoveAnim;
    _pendingMoveAnim = null;

    var els = animItems
      .map(function (item) {
        return {
          el: document.querySelector(
            '.task-slider li[data-id="' + item.id + '"]',
          ),
          delta: item.delta,
        };
      })
      .filter(function (item) {
        return item.el !== null;
      });

    if (els.length === 0) return;

    // FLIP: apply inverse transforms so elements appear to still be in their old positions
    els.forEach(function (item) {
      item.el.style.transition = "none";
      item.el.style.transform = "translateY(" + item.delta + "px)";
    });

    // Force reflow so the browser registers the starting positions
    els[0].el.getBoundingClientRect();

    // Play: animate to natural positions (transition fires together with task-slider)
    els.forEach(function (item) {
      item.el.style.transition = "transform 0.25s cubic-bezier(0.4, 0, 0.2, 1)";
      item.el.style.transform = "";
    });

    setTimeout(function () {
      els.forEach(function (item) {
        item.el.style.transition = "";
      });
    }, 250);
  });

  observer.observe(document.body, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ["data-id"],
  });
}

document.addEventListener("DOMContentLoaded", setupMoveAnimations);

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
