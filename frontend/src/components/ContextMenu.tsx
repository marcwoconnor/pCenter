import { useState, useEffect, useRef } from 'react';

export interface MenuItem {
  label: string;
  icon?: string;
  action: () => void;
  disabled?: boolean;
  danger?: boolean;
  divider?: boolean;
  submenu?: MenuItem[];
}

interface ContextMenuProps {
  x: number;
  y: number;
  items: MenuItem[];
  onClose: () => void;
}

// Submenu component that appears on hover
function SubMenu({ items, onClose, parentRef }: { items: MenuItem[]; onClose: () => void; parentRef: React.RefObject<HTMLDivElement | null> }) {
  const menuRef = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({ left: 0, top: 0 });

  useEffect(() => {
    if (parentRef.current && menuRef.current) {
      const parentRect = parentRef.current.getBoundingClientRect();
      const menuRect = menuRef.current.getBoundingClientRect();
      const viewportWidth = window.innerWidth;
      const viewportHeight = window.innerHeight;

      let left = parentRect.right;
      let top = parentRect.top;

      // If submenu would overflow right, show on left
      if (left + menuRect.width > viewportWidth) {
        left = parentRect.left - menuRect.width;
      }

      // If submenu would overflow bottom, adjust up
      if (top + menuRect.height > viewportHeight) {
        top = viewportHeight - menuRect.height - 10;
      }

      setPosition({ left, top });
    }
  }, [parentRef]);

  return (
    <div
      ref={menuRef}
      className="fixed z-[60] bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg py-1 min-w-[160px] max-h-[300px] overflow-y-auto"
      style={{ left: position.left, top: position.top }}
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
            style={{ backgroundColor: 'transparent' }}
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

// Menu item that can have a submenu
function MenuItemWithSubmenu({ item, onClose }: { item: MenuItem; onClose: () => void }) {
  const [showSubmenu, setShowSubmenu] = useState(false);
  const itemRef = useRef<HTMLDivElement>(null);

  if (item.divider) {
    return <div className="border-t border-gray-200 dark:border-gray-700 my-1" />;
  }

  const hasSubmenu = item.submenu && item.submenu.length > 0;

  return (
    <div
      ref={itemRef}
      onMouseEnter={() => hasSubmenu && setShowSubmenu(true)}
      onMouseLeave={() => hasSubmenu && setShowSubmenu(false)}
      className="relative"
    >
      <button
        onClick={() => {
          if (!item.disabled && !hasSubmenu) {
            item.action();
            onClose();
          }
        }}
        disabled={item.disabled}
        style={{ backgroundColor: 'transparent' }}
        onMouseEnter={(e) => {
          if (!item.disabled) {
            e.currentTarget.style.backgroundColor = item.danger ? 'rgba(239, 68, 68, 0.15)' : 'rgba(59, 130, 246, 0.15)';
          }
        }}
        onMouseLeave={(e) => {
          if (!showSubmenu) {
            e.currentTarget.style.backgroundColor = 'transparent';
          }
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
        <span className="flex-1">{item.label}</span>
        {hasSubmenu && <span className="text-gray-400">▶</span>}
      </button>
      {showSubmenu && item.submenu && (
        <SubMenu items={item.submenu} onClose={onClose} parentRef={itemRef} />
      )}
    </div>
  );
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
        <MenuItemWithSubmenu key={index} item={item} onClose={onClose} />
      ))}
    </div>
  );
}
