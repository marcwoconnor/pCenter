import { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type RFBType = any;

export function ConsolePage() {
  const { type, vmid, name } = useParams<{ type: string; vmid: string; name: string }>();
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<RFBType | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error' | 'closed'>('connecting');
  const [error, setError] = useState<string | null>(null);

  // Set window title
  useEffect(() => {
    document.title = `${name || vmid} - Console`;
  }, [name, vmid]);

  useEffect(() => {
    if (!containerRef.current || !type || !vmid) return;

    let rfb: RFBType = null;
    let mounted = true;

    const connect = async () => {
      try {
        const ticketResp = await fetch(`/api/console/${type}/${vmid}/ticket`);
        if (!ticketResp.ok) {
          throw new Error(`Failed to get ticket: ${ticketResp.statusText}`);
        }
        const { ticket, port } = await ticketResp.json();

        if (!mounted || !containerRef.current) return;

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/api/console/${type}/${vmid}/ws?ticket=${encodeURIComponent(ticket)}&port=${port}`;

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
  }, [type, vmid]);

  const handleContainerClick = () => {
    if (rfbRef.current) {
      rfbRef.current.focus();
    }
  };

  return (
    <div className="h-screen w-screen bg-gray-900 flex flex-col">
      {/* Minimal header */}
      <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700">
        <div className="flex items-center gap-3">
          <span className="text-lg">{type === 'vm' ? '💻' : '📦'}</span>
          <span className="text-white font-medium">{name || `Guest ${vmid}`}</span>
          <span className="text-gray-400 text-sm">({vmid})</span>
          <span className={`px-2 py-0.5 text-xs rounded ${
            status === 'connected' ? 'bg-green-600 text-white' :
            status === 'connecting' ? 'bg-yellow-600 text-white' :
            status === 'error' ? 'bg-red-600 text-white' :
            'bg-gray-600 text-white'
          }`}>
            {status}
          </span>
        </div>
      </div>

      {/* Full-screen VNC */}
      <div className="flex-1 bg-black" onClick={handleContainerClick}>
        {error ? (
          <div className="flex items-center justify-center h-full text-red-400">
            {error}
          </div>
        ) : (
          <div ref={containerRef} className="w-full h-full" />
        )}
      </div>
    </div>
  );
}
