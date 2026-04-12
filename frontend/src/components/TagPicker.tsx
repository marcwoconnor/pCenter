import { useState, useEffect, useRef, useCallback, memo } from 'react';
import { createPortal } from 'react-dom';
import { api } from '../api/client';
import { TagChip } from './TagChip';
import type { Tag } from '../types';

interface TagPickerProps {
  objectType: string;
  objectId: string;
  cluster: string;
  tags: Tag[];                 // Currently assigned tags
  allTags: Tag[];              // All available tags
  onAssign: (tagId: string) => void;
  onUnassign: (tagId: string) => void;
  onTagCreated: (tag: Tag) => void;
}

export const TagPicker = memo(function TagPicker({
  tags: assignedTags, allTags,
  onAssign, onUnassign, onTagCreated,
}: TagPickerProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [newCategory, setNewCategory] = useState('');
  const [newColor, setNewColor] = useState('#3b82f6');
  const [error, setError] = useState<string | null>(null);
  const [dropdownPos, setDropdownPos] = useState<{ top: number; left: number } | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const updatePosition = useCallback(() => {
    if (!buttonRef.current) return;
    const rect = buttonRef.current.getBoundingClientRect();
    setDropdownPos({ top: rect.bottom + 4, left: rect.left });
  }, []);

  // Close on outside click
  useEffect(() => {
    if (!isOpen) return;
    updatePosition();
    const handleClick = (e: MouseEvent) => {
      if (
        dropdownRef.current && !dropdownRef.current.contains(e.target as Node) &&
        buttonRef.current && !buttonRef.current.contains(e.target as Node)
      ) {
        setIsOpen(false);
        setCreating(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    window.addEventListener('scroll', updatePosition, true);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      window.removeEventListener('scroll', updatePosition, true);
    };
  }, [isOpen, updatePosition]);

  const assignedIds = new Set(assignedTags.map(t => t.id));
  const available = allTags.filter(t =>
    !assignedIds.has(t.id) &&
    (search === '' ||
      t.name.toLowerCase().includes(search.toLowerCase()) ||
      t.category.toLowerCase().includes(search.toLowerCase()))
  );

  // Group available tags by category
  const grouped = available.reduce((acc, tag) => {
    const cat = tag.category || 'Uncategorized';
    if (!acc[cat]) acc[cat] = [];
    acc[cat].push(tag);
    return acc;
  }, {} as Record<string, Tag[]>);

  const handleCreate = async () => {
    if (!newName.trim()) return;
    setError(null);
    try {
      const tag = await api.createTag({ category: newCategory, name: newName.trim(), color: newColor });
      onTagCreated(tag);
      onAssign(tag.id);
      setNewName('');
      setNewCategory('');
      setCreating(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create tag');
    }
  };

  const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#f97316'];

  return (
    <div>
      {/* Assigned tags + add button */}
      <div className="flex flex-wrap items-center gap-1.5">
        {assignedTags.map(tag => (
          <TagChip key={tag.id} tag={tag} onRemove={() => onUnassign(tag.id)} />
        ))}
        <button
          ref={buttonRef}
          onClick={() => setIsOpen(!isOpen)}
          className="text-xs px-2 py-0.5 rounded-full border border-dashed border-gray-400 dark:border-gray-600 text-gray-500 dark:text-gray-400 hover:border-blue-500 hover:text-blue-500 transition-colors"
        >
          + Tag
        </button>
      </div>

      {/* Dropdown via portal */}
      {isOpen && dropdownPos && createPortal(
        <div ref={dropdownRef} className="fixed z-[9999] w-64 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 max-h-80 overflow-hidden flex flex-col"
          style={{ top: dropdownPos.top, left: dropdownPos.left }}>
          {/* Search */}
          <div className="p-2 border-b border-gray-200 dark:border-gray-700">
            <input
              type="text"
              placeholder="Search tags..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
              autoFocus
            />
          </div>

          {/* Available tags */}
          <div className="flex-1 overflow-y-auto p-2 space-y-2">
            {Object.entries(grouped).map(([category, catTags]) => (
              <div key={category}>
                <div className="text-xs text-gray-500 font-medium mb-1">{category}</div>
                <div className="space-y-0.5">
                  {catTags.map(tag => (
                    <button
                      key={tag.id}
                      onClick={() => { onAssign(tag.id); }}
                      className="w-full text-left px-2 py-1 text-sm rounded hover:bg-gray-100 dark:hover:bg-gray-700 flex items-center gap-2"
                    >
                      <span className="w-3 h-3 rounded-full flex-shrink-0" style={{ backgroundColor: tag.color }} />
                      <span className="text-gray-900 dark:text-white">{tag.name}</span>
                    </button>
                  ))}
                </div>
              </div>
            ))}
            {available.length === 0 && !creating && (
              <div className="text-sm text-gray-500 text-center py-2">
                {search ? 'No matching tags' : 'All tags assigned'}
              </div>
            )}
          </div>

          {/* Create new tag */}
          <div className="border-t border-gray-200 dark:border-gray-700 p-2">
            {!creating ? (
              <button
                onClick={() => setCreating(true)}
                className="w-full text-left px-2 py-1 text-sm text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20 rounded"
              >
                + Create new tag
              </button>
            ) : (
              <div className="space-y-2">
                <input
                  type="text"
                  placeholder="Tag name"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
                  autoFocus
                />
                <input
                  type="text"
                  placeholder="Category (optional)"
                  value={newCategory}
                  onChange={(e) => setNewCategory(e.target.value)}
                  className="w-full px-2 py-1 text-sm border rounded dark:bg-gray-700 dark:border-gray-600 dark:text-gray-200"
                />
                <div className="flex gap-1">
                  {colors.map(c => (
                    <button
                      key={c}
                      onClick={() => setNewColor(c)}
                      className={`w-5 h-5 rounded-full border-2 ${newColor === c ? 'border-white ring-2 ring-blue-500' : 'border-transparent'}`}
                      style={{ backgroundColor: c }}
                    />
                  ))}
                </div>
                {error && <div className="text-xs text-red-500">{error}</div>}
                <div className="flex gap-1">
                  <button
                    onClick={handleCreate}
                    className="flex-1 px-2 py-1 bg-blue-600 text-white text-sm rounded hover:bg-blue-700"
                  >
                    Create
                  </button>
                  <button
                    onClick={() => { setCreating(false); setError(null); }}
                    className="px-2 py-1 text-sm text-gray-500 hover:text-gray-700 dark:hover:text-gray-300"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>,
        document.body
      )}
    </div>
  );
});
