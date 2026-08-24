import { formatSpeed } from '../../../utils/format';

interface SpeedPoint {
  time: number;
  speed: number;
}

function getNiceMaxScale(maxSpeed: number): number {
  const MB = 1024 * 1024;
  if (maxSpeed <= 1 * MB) return 1 * MB;
  if (maxSpeed <= 2 * MB) return 2 * MB;
  if (maxSpeed <= 5 * MB) return 5 * MB;
  if (maxSpeed <= 10 * MB) return 10 * MB;
  if (maxSpeed <= 20 * MB) return 20 * MB;
  if (maxSpeed <= 50 * MB) return 50 * MB;
  if (maxSpeed <= 100 * MB) return 100 * MB;
  const inMB = Math.ceil(maxSpeed / MB / 25) * 25;
  return inMB * MB;
}

export class SpeedGraph {
  private canvas: HTMLCanvasElement | null = null;
  private ctx: CanvasRenderingContext2D | null = null;
  private points: SpeedPoint[] = [];
  private maxPoints = 18;
  private resizeObserver: ResizeObserver | null = null;

  constructor(canvasId: string) {
    this.canvas = document.getElementById(canvasId) as HTMLCanvasElement | null;
    if (this.canvas) {
      this.ctx = this.canvas.getContext('2d');
      // Initialize with flat initial points
      const now = Date.now();
      for (let i = this.maxPoints; i >= 0; i--) {
        this.points.push({ time: now - i * 2500, speed: 0 });
      }

      // Auto-resize on window / layout change
      if (typeof ResizeObserver !== 'undefined' && this.canvas.parentElement) {
        this.resizeObserver = new ResizeObserver(() => {
          this.render();
        });
        this.resizeObserver.observe(this.canvas.parentElement);
      }

      this.render();
    }
  }

  public addSpeed(speed: number) {
    this.points.push({ time: Date.now(), speed: Math.max(0, speed) });
    if (this.points.length > this.maxPoints) {
      this.points.shift();
    }
    this.render();
  }

  public render() {
    if (!this.canvas || !this.ctx) return;
    const canvas = this.canvas;
    const ctx = this.ctx;

    const rect = canvas.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const width = Math.max(260, rect.width || 280);
    const height = Math.max(140, rect.height || 155);

    if (canvas.width !== Math.round(width * dpr) || canvas.height !== Math.round(height * dpr)) {
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(height * dpr);
    }

    ctx.save();
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    // Padding for axes with generous left space for speed text
    const padL = 64;
    const padR = 14;
    const padT = 16;
    const padB = 26;
    const plotW = width - padL - padR;
    const plotH = height - padT - padB;

    // Calculate clean rounded max scale
    const currentMax = Math.max(...this.points.map((p) => p.speed), 0);
    const topScale = getNiceMaxScale(currentMax);

    // 2 grid steps (Top, Middle, Bottom) for clean non-cluttered vertical space
    const gridSteps = 2;
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.07)';
    ctx.lineWidth = 1;
    ctx.fillStyle = '#94a3b8';
    ctx.font = '10px var(--font-mono, monospace)';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';

    for (let i = 0; i <= gridSteps; i++) {
      const y = padT + (plotH / gridSteps) * i;
      const val = topScale * (1 - i / gridSteps);

      ctx.beginPath();
      ctx.moveTo(padL, y);
      ctx.lineTo(width - padR, y);
      ctx.stroke();

      const label = val === 0 ? '0 B/s' : formatSpeed(val);
      ctx.fillText(label, padL - 8, y);
    }

    if (this.points.length < 2) {
      ctx.restore();
      return;
    }

    // Draw X-axis timestamps
    ctx.textAlign = 'left';
    ctx.textBaseline = 'top';
    const firstTime = new Date(this.points[0].time);
    const lastTime = new Date(this.points[this.points.length - 1].time);
    const formatTime = (d: Date) => d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

    ctx.fillStyle = '#64748b';
    ctx.font = '9px var(--font-mono, monospace)';
    ctx.fillText(formatTime(firstTime), padL, height - padB + 8);
    ctx.textAlign = 'right';
    ctx.fillText(formatTime(lastTime), width - padR, height - padB + 8);

    // Coordinate mapping
    const getX = (idx: number) => padL + (plotW / (this.points.length - 1)) * idx;
    const getY = (val: number) => padT + plotH - (Math.min(val, topScale) / topScale) * plotH;

    // Gradient fill under the curve
    const grad = ctx.createLinearGradient(0, padT, 0, padT + plotH);
    grad.addColorStop(0, 'rgba(16, 185, 129, 0.35)');
    grad.addColorStop(1, 'rgba(16, 185, 129, 0.01)');

    ctx.beginPath();
    ctx.moveTo(getX(0), padT + plotH);
    this.points.forEach((p, idx) => {
      ctx.lineTo(getX(idx), getY(p.speed));
    });
    ctx.lineTo(getX(this.points.length - 1), padT + plotH);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // Line stroke
    ctx.beginPath();
    this.points.forEach((p, idx) => {
      if (idx === 0) ctx.moveTo(getX(idx), getY(p.speed));
      else ctx.lineTo(getX(idx), getY(p.speed));
    });
    ctx.strokeStyle = '#10b981';
    ctx.lineWidth = 2;
    ctx.lineJoin = 'round';
    ctx.stroke();

    // Current point dot
    const lastIdx = this.points.length - 1;
    const lastX = getX(lastIdx);
    const lastY = getY(this.points[lastIdx].speed);

    ctx.beginPath();
    ctx.arc(lastX, lastY, 3.5, 0, Math.PI * 2);
    ctx.fillStyle = '#10b981';
    ctx.fill();
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 1.5;
    ctx.stroke();

    ctx.restore();
  }

  public destroy() {
    this.resizeObserver?.disconnect();
  }
}
