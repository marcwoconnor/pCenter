import { useEffect, useRef } from 'react';

export interface MenuItem {
  label: string;
  icon?: string;
  action: () => void;
  disabled?: boolean;
  danger?: boolean;
  divider?: boolean;
}

interface ContextMenuProps {
  x: number;
  y: number;
  items: MenuItem[];
  onClose: () => void;
}

export function ContextMenu({ x, y, items, onClose }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };

    document.addEventListener('mousedown', handleClickOutside);
    document.addEventListener('keydown', handleEscape);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      document.removeEventListener('keydown', handleEscape);
    };
  }, [onClose]);

  // Adjust position if menu would overflow viewport
  useEffect(() => {
    if (menuRef.current) {
      const rect = menuRef.current.getBoundingClientRect();
      const viewportWidth = window.innerWidth;
      const viewportHeight = window.innerHeight;

      if (rect.right > viewportWidth) {
        menuRef.current.style.left = `${x - rect.width}px`;
      }
      if (rect.bottom > viewportHeight) {
        menuRef.current.style.top = `${y - rect.height}px`;
      }
    }
  }, [x, y]);

  return (
    <div
      ref={menuRef}
      className="fixed z-50 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg py-1 min-w-[160px]"
      style={{ left: x, top: y }}
    >
      {items.map((item, index) => (
        item.divider ? (
          <div key={index} className="border-t border-gray-200 dark:border-gray-700 my-1" />
        ) : (
          <button
            key={index}
            onClick={() => {
              if (!item.disabled) {
                item.action();
                onClose();
              }
            }}
            disabled={item.disabled}
            style={{
              backgroundColor: 'transparent',
            }}
            onMouseEnter={(e) => {
              if (!item.disabled) {
                e.currentTarget.style.backgroundColor = item.danger ? 'rgba(239, 68, 68, 0.15)' : 'rgba(59, 130, 246, 0.15)';
              }
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = 'transparent';
            }}
            className={`w-full px-3 py-2 text-left text-sm flex items-center gap-2 ${
              item.disabled
                ? 'text-gray-400 cursor-not-allowed'
                : item.danger
                  ? 'text-red-600'
                  : 'text-gray-700 dark:text-gray-200'
            }`}
          >
            {item.icon && <span className="w-4 text-center">{item.icon}</span>}
            <span>{item.label}</span>
          </button>
        )
      ))}
    </div>
  );
}
