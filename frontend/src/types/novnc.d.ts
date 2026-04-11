declare module '@novnc/novnc/lib/rfb.js' {
  interface RFBOptions {
    credentials?: {
      password?: string;
      username?: string;
      target?: string;
    };
    shared?: boolean;
    repeaterID?: string;
    wsProtocols?: string[];
  }

  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: RFBOptions);

    // Properties
    scaleViewport: boolean;
    resizeSession: boolean;
    clipViewport: boolean;
    dragViewport: boolean;
    focusOnClick: boolean;
    showDotCursor: boolean;
    background: string;
    qualityLevel: number;
    compressionLevel: number;
    capabilities: { power: boolean };

    // Methods
    disconnect(): void;
    sendCredentials(credentials: { password?: string; username?: string; target?: string }): void;
    sendKey(keysym: number, code: string | null, down?: boolean): void;
    sendCtrlAltDel(): void;
    focus(): void;
    blur(): void;
    machineShutdown(): void;
    machineReboot(): void;
    machineReset(): void;
    clipboardPasteFrom(text: string): void;

    // Event methods
    addEventListener(type: 'connect', listener: () => void): void;
    addEventListener(type: 'disconnect', listener: (e: CustomEvent<{ clean: boolean }>) => void): void;
    addEventListener(type: 'credentialsrequired', listener: () => void): void;
    addEventListener(type: 'securityfailure', listener: (e: CustomEvent<{ status: number; reason: string }>) => void): void;
    addEventListener(type: 'clipboard', listener: (e: CustomEvent<{ text: string }>) => void): void;
    addEventListener(type: 'bell', listener: () => void): void;
    addEventListener(type: 'desktopname', listener: (e: CustomEvent<{ name: string }>) => void): void;
    addEventListener(type: 'capabilities', listener: (e: CustomEvent<{ capabilities: { power: boolean } }>) => void): void;
    removeEventListener(type: string, listener: EventListener): void;
  }
}
