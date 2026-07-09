import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { Bell, CheckCircle2 } from 'lucide-react';
import { useAuth } from '../contexts/AuthContext';
import { fetchNotifications, fetchUnreadNotificationCount, markNotificationRead, markAllNotificationsRead } from '../lib/api';
import type { Notification } from '../types/notification';

export const NotificationDropdown: React.FC = () => {
  const { token, user } = useAuth();
  const navigate = useNavigate();
  const [isOpen, setIsOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  const loadUnreadCount = useCallback(async () => {
    if (!token) return;
    try {
      const data = await fetchUnreadNotificationCount(token);
      setUnreadCount(data.count);
    } catch (error) {
      console.error('Failed to fetch unread count', error);
    }
  }, [token]);

  useEffect(() => {
    if (token) {
      loadUnreadCount();
      // Optional: Polling every 3 minutes
      const intervalId = setInterval(loadUnreadCount, 3 * 60 * 1000);
      return () => clearInterval(intervalId);
    }
  }, [token, loadUnreadCount]);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const loadNotifications = async () => {
    setIsLoading(true);
    try {
      const data = await fetchNotifications(token!, 1, 10);
      setNotifications(Array.isArray(data.data) ? data.data : []);
    } catch (error) {
      console.error('Failed to fetch notifications', error);
      setNotifications([]);
    } finally {
      setIsLoading(false);
    }
  };

  const toggleDropdown = () => {
    if (!isOpen) {
      loadNotifications();
    }
    setIsOpen(!isOpen);
  };

  const handleMarkAllRead = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await markAllNotificationsRead(token!);
      setUnreadCount(0);
      setNotifications(notifications.map(n => ({ ...n, read_at: new Date().toISOString() })));
    } catch (error) {
      console.error('Failed to mark all as read', error);
    }
  };

  const handleNotificationClick = async (notification: Notification) => {
    if (!notification.read_at) {
      try {
        await markNotificationRead(token!, notification.id);
        setUnreadCount(Math.max(0, unreadCount - 1));
        setNotifications(notifications.map(n => n.id === notification.id ? { ...n, read_at: new Date().toISOString() } : n));
      } catch (error) {
        console.error('Failed to mark as read', error);
      }
    }

    setIsOpen(false);

    // Navigation logic
    if (notification.entity_type === 'BOOKING' && notification.entity_id) {
      if (user?.role === 'OWNER' || user?.role === 'STAFF') {
        navigate(`/owner/bookings?q=${notification.entity_id.substring(0, 8)}`);
      } else {
        navigate(`/bookings/${notification.entity_id}`);
      }
    } else if (notification.entity_type === 'REFUND') {
      if (user?.role === 'OWNER' || user?.role === 'STAFF') {
        navigate('/owner/refunds');
      } else {
        navigate('/bookings');
      }
    }
  };

  return (
    <div className="relative" ref={dropdownRef}>
      <button 
        onClick={toggleDropdown}
        className="w-10 h-10 rounded-full flex items-center justify-center text-text-muted hover:bg-gray-100 hover:text-text-main transition-colors relative"
      >
        <Bell className="w-5 h-5" />
        {unreadCount > 0 && (
          <span className="absolute top-2 right-2.5 flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
            <span className="relative inline-flex rounded-full h-2 w-2 bg-red-500"></span>
          </span>
        )}
      </button>

      {isOpen && (
        <div className="absolute right-0 mt-2 w-80 sm:w-96 bg-white rounded-2xl shadow-xl border border-border-main overflow-hidden z-50">
          <div className="p-4 border-b border-border-main flex justify-between items-center bg-gray-50/50">
            <h3 className="font-bold text-text-main">Notifikasi</h3>
            {unreadCount > 0 && (
              <button 
                onClick={handleMarkAllRead}
                className="text-[13px] font-semibold text-primary hover:text-primary-hover flex items-center gap-1"
              >
                <CheckCircle2 className="w-4 h-4" />
                Tandai semua dibaca
              </button>
            )}
          </div>
          
          <div className="max-h-[400px] overflow-y-auto">
            {isLoading ? (
              <div className="p-8 text-center text-text-muted text-sm font-medium">Memuat notifikasi...</div>
            ) : notifications.length === 0 ? (
              <div className="p-8 text-center text-text-muted text-sm font-medium">Belum ada notifikasi.</div>
            ) : (
              <div className="flex flex-col">
                {notifications.map((notif) => (
                  <div 
                    key={notif.id}
                    onClick={() => handleNotificationClick(notif)}
                    className={`p-4 border-b border-border-main/50 hover:bg-gray-50 cursor-pointer transition-colors ${!notif.read_at ? 'bg-primary/5' : ''}`}
                  >
                    <div className="flex justify-between items-start mb-1">
                      <h4 className={`text-[14px] ${!notif.read_at ? 'font-bold text-text-main' : 'font-semibold text-text-secondary'}`}>
                        {notif.title}
                      </h4>
                      {!notif.read_at && (
                        <span className="w-2 h-2 rounded-full bg-primary shrink-0 mt-1.5" />
                      )}
                    </div>
                    <p className={`text-[13px] ${!notif.read_at ? 'text-text-main font-medium' : 'text-text-muted'} line-clamp-2`}>
                      {notif.message}
                    </p>
                    <div className="text-[11px] text-text-muted mt-2 font-medium">
                      {new Date(notif.created_at).toLocaleDateString('id-ID', { 
                        day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' 
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          {notifications.length > 0 && (
            <div className="p-3 border-t border-border-main bg-gray-50/50 text-center">
              <span className="text-[12px] text-text-muted font-medium">Menampilkan 10 notifikasi terbaru</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
