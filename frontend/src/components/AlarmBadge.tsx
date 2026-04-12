import { useState, useRef, useEffect, memo } from 'react';
import { useCluster } from '../context/ClusterContext';
import { api } from '../api/client';

export const AlarmBadge = memo(function AlarmBadge() {
  const { alarms } = useCluster();
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const criticalCount = alarms.filter(a => a.state === 'critical').length;
  const warningCount = alarms.filter(a => a.state === 'warning').length;
  const totalCount = alarms.length;

  useEffect(() => {
    if (!isOpen) return;
    const handleClick = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [isOpen]);

  const handleAcknowledge = async (alarmId: string) => {
    try {
      await api.acknowledgeAlarm(alarmId, 'admin');
    } catch {}
  };

  const badgeColor = criticalCount > 0
    ? 'bg-red-500'
    : warningCount > 0
    ? 'bg-yellow-500'
    : 'bg-gray-500';

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="relative p-2 text-gray-400 hover:text-white transition-colors"
        title={`${totalCount} active alarm${totalCount !== 1 ? 's' : ''}`}
      >
        {/* Bell icon */}
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
        </svg>
        {totalCount > 0 && (
          <span className={`absolute -top-0.5 -right-0.5 inline-flex items-center justify-center w-4 h-4 text-[10px] font-bold text-white rounded-full ${badgeColor}`}>
            {totalCount > 9 ? '9+' : totalCount}
          </span>
        )}
      </button>

      {isOpen && (
        <div className="absolute right-0 mt-2 w-80 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 z-50 max-h-96 overflow-hidden flex flex-col">
          <div className="px-4 py-2 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
            <span className="font-medium text-gray-900 dark:text-white text-sm">
              Active Alarms ({totalCount})
            </span>
            {criticalCount > 0 && (
              <span className="px-2 py-0.5 bg-red-500 text-white text-xs rounded-full">
                {criticalCount} critical
              </span>
            )}
          </div>

          <div className="flex-1 overflow-y-auto">
            {totalCount === 0 ? (
              <div className="p-4 text-center text-gray-500 text-sm">
                No active alarms
              </div>
            ) : (
              <div className="divide-y divide-gray-200 dark:divide-gray-700">
                {alarms.slice(0, 10).map(alarm => (
                  <div key={alarm.id} className="px-4 py-2 hover:bg-gray-50 dark:hover:bg-gray-700/50">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className={`w-2 h-2 rounded-full flex-shrink-0 ${
                            alarm.state === 'critical' ? 'bg-red-500' : 'bg-yellow-500'
                          }`} />
                          <span className="text-sm font-medium text-gray-900 dark:text-white truncate">
                            {alarm.definition_name}
                          </span>
                        </div>
                        <div className="text-xs text-gray-500 mt-0.5">
                          {alarm.resource_name || alarm.resource_id} &middot; {alarm.current_value.toFixed(1)}%
                        </div>
                      </div>
                      {!alarm.acknowledged_by && (
                        <button
                          onClick={() => handleAcknowledge(alarm.id)}
                          className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 flex-shrink-0"
                        >
                          Ack
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
});
