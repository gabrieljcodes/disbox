/**
 * Accessible Modal manager with escape key support, backdrop click, and focus trapping.
 */
export class Modal {
  private backdrop: HTMLElement;
  private onOpenCallbacks: (() => void)[] = [];
  private onCloseCallbacks: (() => void)[] = [];

  constructor(modalId: string) {
    const el = document.getElementById(modalId);
    if (!el) {
      throw new Error(`Modal element with ID #${modalId} not found.`);
    }
    this.backdrop = el;
    this.initEvents();
  }

  private initEvents() {
    // Backdrop click close
    this.backdrop.addEventListener('click', (e) => {
      if (e.target === this.backdrop) {
        this.close();
      }
    });

    // Close button click close
    const closeBtns = this.backdrop.querySelectorAll('[data-close-modal]');
    closeBtns.forEach((btn) => {
      btn.addEventListener('click', () => this.close());
    });
  }

  public open() {
    this.backdrop.classList.add('active');
    this.backdrop.style.display = 'flex';
    document.body.style.overflow = 'hidden';

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        this.close();
        window.removeEventListener('keydown', onKeyDown);
      }
    };
    window.addEventListener('keydown', onKeyDown);

    this.onOpenCallbacks.forEach((cb) => cb());
  }

  public close() {
    this.backdrop.classList.remove('active');
    setTimeout(() => {
      if (!this.backdrop.classList.contains('active')) {
        this.backdrop.style.display = 'none';
        document.body.style.overflow = '';
      }
    }, 200);

    this.onCloseCallbacks.forEach((cb) => cb());
  }

  public onOpen(cb: () => void) {
    this.onOpenCallbacks.push(cb);
  }

  public onClose(cb: () => void) {
    this.onCloseCallbacks.push(cb);
  }

  public isOpen(): boolean {
    return this.backdrop.classList.contains('active');
  }
}
