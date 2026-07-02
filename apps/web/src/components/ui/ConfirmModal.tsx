import React from 'react';
import { X, AlertTriangle } from 'lucide-react';
import { Button } from './Button';

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: React.ReactNode;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
  isLoading?: boolean;
  isDestructive?: boolean;
}

export const ConfirmModal: React.FC<ConfirmModalProps> = ({
  isOpen,
  title,
  message,
  confirmText = 'Konfirmasi',
  cancelText = 'Batal',
  onConfirm,
  onCancel,
  isLoading = false,
  isDestructive = false,
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="bg-white rounded-2xl w-full max-w-sm overflow-hidden shadow-2xl animate-in zoom-in-95 duration-200">
        <div className="p-6">
          <div className="flex justify-between items-start mb-4">
            <div className={`p-3 rounded-full ${isDestructive ? 'bg-red-100 text-red-600' : 'bg-primary/10 text-primary'}`}>
              <AlertTriangle className="w-6 h-6" />
            </div>
            <button 
              onClick={onCancel}
              disabled={isLoading}
              className="text-gray-400 hover:text-gray-600 transition-colors p-1"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
          
          <h3 className="text-xl font-bold text-text-main mb-2">{title}</h3>
          <div className="text-text-muted text-sm leading-relaxed mb-6">
            {message}
          </div>

          <div className="flex gap-3">
            <Button 
              type="button" 
              variant="outline" 
              className="flex-1" 
              onClick={onCancel}
              disabled={isLoading}
            >
              {cancelText}
            </Button>
            <Button 
              type="button" 
              className={`flex-1 ${isDestructive ? 'bg-red-600 hover:bg-red-700 text-white border-none' : ''}`}
              onClick={onConfirm}
              disabled={isLoading}
            >
              {isLoading ? 'Memproses...' : confirmText}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
