import { useEffect, useRef, useState } from 'react';
import { Rnd } from 'react-rnd';
import type { ConsoleWindow } from '../context/ClusterContext';

interface ConsoleProps {
  console: ConsoleWindow;
  onClose: () => void;
  onFocus: () => void;
  onUpdate: (updates: Partial<ConsoleWindow>) => void;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type RFBType = any;

export function Console({ console: win, onClose, onFocus, onUpdate }: ConsoleProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFBType | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error' | 'closed'>('connecting');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    let rfb: RFBType = null;
    let mounted = true;

    const connect = async () => {
      try {
        const ticketResp = await fetch(`/api/console/${win.type}/${win.vmid}/ticket`);
        if (!ticketResp.ok) {
          throw new Error(`Failed to get ticket: ${ticketResp.statusText}`);
        }
        const { ticket, port } = await ticketResp.json();

        if (!mounted || !containerRef.current) return;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/console/${win.type}/${win.vmid}/ws?ticket=${encodeURIComponent(ticket)}&port=${port}`;

        const module = await import('@novnc/novnc/lib/rfb.js');
        if (!mounted || !containerRef.current) return;

        const RFB = module.default;

        rfb = new RFB(containerRef.current, wsUrl, {
          credentials: { password: ticket },
        });

        rfbRef.current = rfb;
        rfb.scaleViewport = true;
        rfb.resizeSession = false;

        rfb.addEventListener('connect', () => {
          if (mounted) {
            setStatus('connected');
            rfb.focus();
          }
        });

        rfb.addEventListener('disconnect', (e: CustomEvent) => {
          if (mounted) {
            if (e.detail.clean) {
              setStatus('closed');
            } else {
              setStatus('error');
              setError('Connection lost');
            }
          }
        });

        rfb.addEventListener('securityfailure', (e: CustomEvent) => {
          if (mounted) {
            setStatus('error');
            setError(`Security error: ${e.detail.reason}`);
          }
        });
      } catch (err) {
        if (mounted) {
          setStatus('error');
          setError(err instanceof Error ? err.message : 'Failed to initialize VNC');
        }
      }
    };

    connect();

    return () => {
      mounted = false;
      if (rfbRef.current) {
        rfbRef.current.disconnect();
        rfbRef.current = null;
      }
    };
  }, [win.type, win.vmid]);

  const handleContainerClick = () => {
    if (rfbRef.current) {
      rfbRef.current.focus();
    }
  };

  return (
    <Rnd
      default={{
        x: win.x,
        y: win.y,
        width: win.width,
        height: win.height,
      }}
      minWidth={400}
      minHeight={300}
      bounds="window"
      style={{ zIndex: win.zIndex }}
      dragHandleClassName="console-drag-handle"
      onDragStart={onFocus}
      onResizeStart={onFocus}
      onDragStop={(_e, d) => onUpdate({ x: d.x, y: d.y })}
      onResizeStop={(_e, _dir, ref, _delta, pos) => {
        onUpdate({
          width: parseInt(ref.style.width),
          height: parseInt(ref.style.height),
          x: pos.x,
          y: pos.y,
        });
      }}
    >
      <div
        className="bg-gray-900 rounded-lg shadow-2xl flex flex-col h-full border border-gray-700"
        onMouseDown={onFocus}
      >
        {/* Draggable Header */}
        <div className="console-drag-handle flex items-center justify-between px-4 py-2 bg-gray-800 rounded-t-lg border-b border-gray-700 cursor-move select-none">
          <div className="flex items-center gap-3">
            <span className="text-lg">{win.type === 'vm' ? '💻' : '📦'}</span>
            <span className="text-white font-medium">{win.name}</span>
            <span className="text-gray-400 text-sm">({win.vmid})</span>
            <span className={`px-2 py-0.5 text-xs rounded ${
              status === 'connected' ? 'bg-green-600 text-white' :
              status === 'connecting' ? 'bg-yellow-600 text-white' :
              status === 'error' ? 'bg-red-600 text-white' :
              'bg-gray-600 text-white'
            }`}>
              {status}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => {
                // Open in new window and close this one
                const url = `/console/${win.type}/${win.vmid}/${encodeURIComponent(win.name)}`;
                window.open(url, `console-${win.vmid}`, 'width=1024,height=768');
                onClose();
              }}
              className="text-gray-400 hover:text-white hover:bg-blue-600 px-2 py-1 rounded"
              title="Pop out to new window"
            >
              ⧉
            </button>
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-white hover:bg-red-600 px-2 py-1 rounded"
              title="Close"
            >
              ✕
            </button>
          </div>
        </div>

        {/* VNC container */}
        <div className="flex-1 overflow-hidden bg-black" onClick={handleContainerClick}>
          {error ? (
            <div className="flex items-center justify-center h-full text-red-400">
              {error}
            </div>
          ) : (
            <div ref={containerRef} className="w-full h-full" />
          )}
        </div>
      </div>
    </Rnd>
  );
}
