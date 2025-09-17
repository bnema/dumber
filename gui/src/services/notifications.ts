import type { NotificationOptions } from '../types/app.js';

interface TimerElement extends HTMLElement {
  timeout?: ReturnType<typeof setTimeout>;
}

export class NotificationService {
  private zoomNotification: HTMLElement | null = null;
  private generalNotification: HTMLElement | null = null;

  showZoom(message: string, duration = 2000): void {
    // Create or update zoom notification
    if (!this.zoomNotification) {
      this.zoomNotification = this.createNotificationElement('zoom-notification', {
        top: '20px',
        right: '20px',
        background: 'rgba(0, 0, 0, 0.8)',
        color: 'white'
      });
    }

    this.updateNotification(this.zoomNotification, message, duration);
  }

  show(message: string, type: NotificationOptions['type'] = 'info', duration = 3000): void {
    // Create or update general notification
    if (!this.generalNotification) {
      this.generalNotification = this.createNotificationElement('general-notification', {
        top: '60px',
        right: '20px'
      });
    }

    // Set colors based on type
    const colors = this.getNotificationColors(type);
    this.generalNotification.style.background = colors.background;
    this.generalNotification.style.color = colors.color;

    this.updateNotification(this.generalNotification, message, duration);
  }

  private createNotificationElement(id: string, baseStyles: Record<string, string>): HTMLElement {
    const notification = document.createElement('div');
    notification.id = id;
    
    const styles = {
      position: 'fixed',
      padding: '10px 15px',
      borderRadius: '5px',
      fontSize: '14px',
      zIndex: '10000',
      transition: 'opacity 0.3s',
      opacity: '0',
      ...baseStyles
    };

    notification.style.cssText = Object.entries(styles)
      .map(([key, value]) => `${this.camelToKebab(key)}: ${value}`)
      .join('; ');

    document.body.appendChild(notification);
    return notification;
  }

  private updateNotification(element: HTMLElement, message: string, duration: number): void {
    element.textContent = message;
    element.style.opacity = '1';

    // Clear any existing timeout
    const timerElement = element as TimerElement;
    if (timerElement.timeout) {
      clearTimeout(timerElement.timeout);
    }

    // Hide after duration
    timerElement.timeout = setTimeout(() => {
      element.style.opacity = '0';
    }, duration);
  }

  private getNotificationColors(type: NotificationOptions['type']): { background: string; color: string } {
    switch (type) {
      case 'error':
        return { background: 'rgba(220, 53, 69, 0.9)', color: 'white' };
      case 'success':
        return { background: 'rgba(25, 135, 84, 0.9)', color: 'white' };
      default:
        return { background: 'rgba(13, 110, 253, 0.9)', color: 'white' };
    }
  }

  private camelToKebab(str: string): string {
    return str.replace(/[A-Z]/g, letter => `-${letter.toLowerCase()}`);
  }
}