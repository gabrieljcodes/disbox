document.addEventListener('DOMContentLoaded', () => {
  const viewport = document.getElementById('player-viewport');
  const activeType = viewport?.getAttribute('data-active-type') || '';

  // Playlist Navigation
  const playlistLinks = Array.from(document.querySelectorAll<HTMLAnchorElement>('.playlist-item'));
  const activeIndex = playlistLinks.findIndex((el) => el.classList.contains('active'));

  function goPrev() {
    if (activeIndex > 0) {
      window.location.href = playlistLinks[activeIndex - 1].href;
    }
  }

  function goNext() {
    if (activeIndex >= 0 && activeIndex < playlistLinks.length - 1) {
      window.location.href = playlistLinks[activeIndex + 1].href;
    }
  }

  if (activeType === 'image') {
    initImageViewer(goPrev, goNext, activeIndex, playlistLinks.length);
  }
});

function initImageViewer(
  goPrev: () => void,
  goNext: () => void,
  activeIndex: number,
  totalItems: number
) {
  const container = document.getElementById('image-viewport');
  const img = document.getElementById('image-preview') as HTMLImageElement | null;
  const controls = document.getElementById('img-floating-controls');
  const zoomResetBtn = document.getElementById('btn-zoom-reset');
  const btnPrev = document.getElementById('btn-img-prev');
  const btnNext = document.getElementById('btn-img-next');
  const btnZoomIn = document.getElementById('btn-zoom-in');
  const btnZoomOut = document.getElementById('btn-zoom-out');

  if (!container || !img || !controls) return;

  let scale = 1;
  let pointX = 0;
  let pointY = 0;
  let isPanning = false;
  let startX = 0;
  let startY = 0;
  let renderScheduled = false;

  // Auto-hide floating controls
  let hideTimeout: ReturnType<typeof setTimeout> | null = null;
  container.addEventListener('mousemove', () => {
    controls.classList.add('visible');
    if (hideTimeout) clearTimeout(hideTimeout);
    hideTimeout = setTimeout(() => {
      controls.classList.remove('visible');
    }, 2000);
  });

  function applyTransform() {
    if (!renderScheduled) {
      renderScheduled = true;
      requestAnimationFrame(() => {
        if (img) {
          img.style.transform = `translate(${pointX}px, ${pointY}px) scale(${scale})`;
        }
        if (zoomResetBtn) {
          zoomResetBtn.textContent = `${Math.round(scale * 100)}%`;
        }
        renderScheduled = false;
      });
    }
  }

  // Mouse pan events
  container.addEventListener('mousedown', (e: MouseEvent) => {
    if (e.button !== 0) return;
    e.preventDefault();
    startX = e.clientX - pointX;
    startY = e.clientY - pointY;
    isPanning = true;
  });

  window.addEventListener('mouseup', () => {
    isPanning = false;
  });

  window.addEventListener('mousemove', (e: MouseEvent) => {
    if (!isPanning) return;
    e.preventDefault();
    pointX = e.clientX - startX;
    pointY = e.clientY - startY;
    applyTransform();
  });

  // Wheel zoom centered at mouse cursor
  container.addEventListener(
    'wheel',
    (e: WheelEvent) => {
      e.preventDefault();
      const delta = e.deltaY < 0 ? 1.15 : 1 / 1.15;
      const newScale = Math.min(Math.max(0.1, scale * delta), 15);

      if (newScale !== scale) {
        const rect = container.getBoundingClientRect();
        const mouseX = e.clientX - rect.left - rect.width / 2;
        const mouseY = e.clientY - rect.top - rect.height / 2;
        const ratio = newScale / scale;

        pointX = mouseX - (mouseX - pointX) * ratio;
        pointY = mouseY - (mouseY - pointY) * ratio;
        scale = newScale;
        applyTransform();
      }
    },
    { passive: false }
  );

  // Button handlers
  btnZoomIn?.addEventListener('click', () => {
    scale = Math.min(scale * 1.4, 15);
    pointX *= 1.4;
    pointY *= 1.4;
    applyTransform();
  });

  btnZoomOut?.addEventListener('click', () => {
    scale = Math.max(scale / 1.4, 0.1);
    pointX /= 1.4;
    pointY /= 1.4;
    applyTransform();
  });

  zoomResetBtn?.addEventListener('click', () => {
    scale = 1;
    pointX = 0;
    pointY = 0;
    applyTransform();
  });

  if (btnPrev) {
    btnPrev.addEventListener('click', goPrev);
    if (activeIndex <= 0) {
      btnPrev.style.opacity = '0.3';
      btnPrev.style.pointerEvents = 'none';
    }
  }

  if (btnNext) {
    btnNext.addEventListener('click', goNext);
    if (activeIndex >= totalItems - 1 || activeIndex === -1) {
      btnNext.style.opacity = '0.3';
      btnNext.style.pointerEvents = 'none';
    }
  }

  // Keyboard Navigation & Shortcuts
  window.addEventListener('keydown', (e: KeyboardEvent) => {
    const panStep = 60;
    if (e.key === 'ArrowLeft') {
      if (scale > 1) {
        pointX += panStep;
        applyTransform();
      } else {
        goPrev();
      }
    } else if (e.key === 'ArrowRight') {
      if (scale > 1) {
        pointX -= panStep;
        applyTransform();
      } else {
        goNext();
      }
    } else if (e.key === 'ArrowUp' && scale > 1) {
      pointY += panStep;
      applyTransform();
    } else if (e.key === 'ArrowDown' && scale > 1) {
      pointY -= panStep;
      applyTransform();
    } else if (e.key === '+' || e.key === '=') {
      scale = Math.min(scale * 1.25, 15);
      pointX *= 1.25;
      pointY *= 1.25;
      applyTransform();
    } else if (e.key === '-' || e.key === '_') {
      scale = Math.max(scale / 1.25, 0.1);
      pointX /= 1.25;
      pointY /= 1.25;
      applyTransform();
    } else if (e.key === '0' || e.key === 'Escape') {
      scale = 1;
      pointX = 0;
      pointY = 0;
      applyTransform();
    }
  });
}
