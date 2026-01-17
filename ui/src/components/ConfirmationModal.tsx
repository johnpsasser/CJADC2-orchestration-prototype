import { useEffect, useRef } from 'react';
import clsx from 'clsx';

export type ConfirmationVariant = 'danger' | 'warning' | 'info';

export interface ConfirmationModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: ConfirmationVariant;
  icon?: React.ReactNode;
  isLoading?: boolean;
  details?: string[];
}

const variantStyles: Record<ConfirmationVariant, {
  iconBg: string;
  iconColor: string;
  confirmBtn: string;
  borderColor: string;
  glowColor: string;
}> = {
  danger: {
    iconBg: 'bg-red-900/50',
    iconColor: 'text-red-400',
    confirmBtn: 'bg-red-600 hover:bg-red-700 focus:ring-red-500',
    borderColor: 'border-red-900/50',
    glowColor: 'shadow-red-900/20',
  },
  warning: {
    iconBg: 'bg-orange-900/50',
    iconColor: 'text-orange-400',
    confirmBtn: 'bg-orange-600 hover:bg-orange-700 focus:ring-orange-500',
    borderColor: 'border-orange-900/50',
    glowColor: 'shadow-orange-900/20',
  },
  info: {
    iconBg: 'bg-blue-900/50',
    iconColor: 'text-blue-400',
    confirmBtn: 'bg-blue-600 hover:bg-blue-700 focus:ring-blue-500',
    borderColor: 'border-blue-900/50',
    glowColor: 'shadow-blue-900/20',
  },
};

// Default icons for each variant
function DefaultIcon({ variant }: { variant: ConfirmationVariant }) {
  switch (variant) {
    case 'danger':
      return (
        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
          />
        </svg>
      );
    case 'warning':
      return (
        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
          />
        </svg>
      );
    case 'info':
      return (
        <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
          />
        </svg>
      );
  }
}

export function ConfirmationModal({
  isOpen,
  onClose,
  onConfirm,
  title,
  message,
  confirmText = 'Confirm',
  cancelText = 'Cancel',
  variant = 'danger',
  icon,
  isLoading = false,
  details,
}: ConfirmationModalProps) {
  const modalRef = useRef<HTMLDivElement>(null);
  const confirmBtnRef = useRef<HTMLButtonElement>(null);
  const styles = variantStyles[variant];

  // Focus trap and escape key handling
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isLoading) {
        onClose();
      }
    };

    // Focus the cancel button when modal opens (safer default)
    const timer = setTimeout(() => {
      confirmBtnRef.current?.focus();
    }, 50);

    document.addEventListener('keydown', handleKeyDown);
    document.body.style.overflow = 'hidden';

    return () => {
      clearTimeout(timer);
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = 'unset';
    };
  }, [isOpen, onClose, isLoading]);

  // Click outside to close
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget && !isLoading) {
      onClose();
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
    >
      {/* Backdrop with blur */}
      <div className="absolute inset-0 bg-gray-950/80 backdrop-blur-sm" />

      {/* Modal */}
      <div
        ref={modalRef}
        className={clsx(
          'relative w-full max-w-md mx-4 rounded-xl overflow-hidden',
          'bg-gray-900 border shadow-2xl',
          styles.borderColor,
          styles.glowColor,
          'animate-modal-enter'
        )}
      >
        {/* Header stripe */}
        <div
          className={clsx(
            'h-1 w-full',
            variant === 'danger' && 'bg-gradient-to-r from-red-600 via-red-500 to-red-600',
            variant === 'warning' && 'bg-gradient-to-r from-orange-600 via-orange-500 to-orange-600',
            variant === 'info' && 'bg-gradient-to-r from-blue-600 via-blue-500 to-blue-600'
          )}
        />

        <div className="p-6">
          {/* Icon and Title */}
          <div className="flex items-start gap-4">
            <div
              className={clsx(
                'flex-shrink-0 w-12 h-12 rounded-full flex items-center justify-center',
                styles.iconBg,
                styles.iconColor
              )}
            >
              {icon || <DefaultIcon variant={variant} />}
            </div>

            <div className="flex-1 min-w-0">
              <h3
                id="modal-title"
                className="text-lg font-semibold text-gray-100 leading-tight"
              >
                {title}
              </h3>
              <p className="mt-2 text-sm text-gray-400 leading-relaxed">
                {message}
              </p>

              {/* Details list */}
              {details && details.length > 0 && (
                <div className="mt-4 p-3 bg-gray-800/50 rounded-lg border border-gray-700/50">
                  <p className="text-xs font-medium text-gray-500 uppercase tracking-wide mb-2">
                    Affected Items
                  </p>
                  <ul className="space-y-1">
                    {details.map((detail, index) => (
                      <li
                        key={index}
                        className="flex items-center gap-2 text-sm text-gray-300"
                      >
                        <span className={clsx('w-1.5 h-1.5 rounded-full', styles.iconColor.replace('text-', 'bg-'))} />
                        {detail}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          </div>

          {/* Actions */}
          <div className="mt-6 flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={isLoading}
              className={clsx(
                'px-4 py-2.5 text-sm font-medium rounded-lg',
                'bg-gray-800 border border-gray-700 text-gray-300',
                'hover:bg-gray-700 hover:text-gray-200',
                'focus:outline-none focus:ring-2 focus:ring-gray-500 focus:ring-offset-2 focus:ring-offset-gray-900',
                'transition-colors duration-150',
                isLoading && 'opacity-50 cursor-not-allowed'
              )}
            >
              {cancelText}
            </button>

            <button
              ref={confirmBtnRef}
              type="button"
              onClick={onConfirm}
              disabled={isLoading}
              className={clsx(
                'px-4 py-2.5 text-sm font-medium rounded-lg text-white',
                'focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-gray-900',
                'transition-colors duration-150',
                'flex items-center gap-2 min-w-[100px] justify-center',
                styles.confirmBtn,
                isLoading && 'opacity-75 cursor-wait'
              )}
            >
              {isLoading ? (
                <>
                  <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                  <span>Processing...</span>
                </>
              ) : (
                confirmText
              )}
            </button>
          </div>
        </div>

        {/* Close button */}
        <button
          type="button"
          onClick={onClose}
          disabled={isLoading}
          className={clsx(
            'absolute top-4 right-4 p-1 rounded-lg',
            'text-gray-500 hover:text-gray-300 hover:bg-gray-800',
            'focus:outline-none focus:ring-2 focus:ring-gray-500',
            'transition-colors duration-150',
            isLoading && 'opacity-50 cursor-not-allowed'
          )}
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </div>
  );
}

export default ConfirmationModal;
