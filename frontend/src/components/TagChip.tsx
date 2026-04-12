import { memo } from 'react';
import type { Tag } from '../types';

interface TagChipProps {
  tag: Tag;
  onRemove?: () => void;
  size?: 'sm' | 'md';
}

export const TagChip = memo(function TagChip({ tag, onRemove, size = 'sm' }: TagChipProps) {
  const sizeClasses = size === 'sm' ? 'text-xs px-1.5 py-0.5' : 'text-sm px-2 py-1';

  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full font-medium ${sizeClasses}`}
      style={{
        backgroundColor: tag.color + '20', // 12% opacity background
        color: tag.color,
        border: `1px solid ${tag.color}40`,
      }}
    >
      {tag.category && (
        <span className="opacity-60">{tag.category}:</span>
      )}
      {tag.name}
      {onRemove && (
        <button
          onClick={(e) => { e.stopPropagation(); onRemove(); }}
          className="ml-0.5 hover:opacity-100 opacity-60 font-bold"
          title="Remove tag"
        >
          x
        </button>
      )}
    </span>
  );
});
