import { useState, type FormEvent } from 'react';
import { authApi } from '../api/auth';
import type { TOTPSetupResponse } from '../types/auth';

interface TOTPSetupWizardProps {
  onComplete: () => void;
  onCancel: () => void;
}

export function TOTPSetupWizard({ onComplete, onCancel }: TOTPSetupWizardProps) {
  const [step, setStep] = useState<'setup' | 'verify' | 'codes'>('setup');
  const [setupData, setSetupData] = useState<TOTPSetupResponse | null>(null);
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [codesConfirmed, setCodesConfirmed] = useState(false);

  const handleSetup = async () => {
    setIsLoading(true);
    setError(null);

    try {
      const response = await authApi.setupTOTP();
      setSetupData(response);
      setStep('verify');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to set up 2FA');
    } finally {
      setIsLoading(false);
    }
  };

  const handleVerify = async (e: FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError(null);

    const normalizedCode = code.replace(/\s|-/g, '');
    if (normalizedCode.length !== 6) {
      setError('Please enter a 6-digit code');
      setIsLoading(false);
      return;
    }

    try {
      await authApi.verifyTOTPSetup({ code: normalizedCode });
      setStep('codes');
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Invalid code');
    } finally {
      setIsLoading(false);
    }
  };

  const handleComplete = () => {
    if (!codesConfirmed) {
      setError('Please confirm you have saved your recovery codes');
      return;
    }
    onComplete();
  };

  // Step 1: Initial setup prompt
  if (step === 'setup') {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-slate-800 rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
          <h2 className="text-xl font-bold text-white mb-4">
            Enable Two-Factor Authentication
          </h2>
          <p className="text-slate-300 mb-6">
            Two-factor authentication adds an extra layer of security to your
            account. You'll need an authenticator app like Google Authenticator,
            Authy, or 1Password.
          </p>

          {error && (
            <div className="bg-red-500/10 border border-red-500/50 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          <div className="flex gap-3">
            <button
              onClick={onCancel}
              className="flex-1 py-2 px-4 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSetup}
              disabled={isLoading}
              className="flex-1 py-2 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-600/50 text-white rounded-lg transition-colors"
            >
              {isLoading ? 'Setting up...' : 'Continue'}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Step 2: QR code and verification
  if (step === 'verify' && setupData) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-slate-800 rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
          <h2 className="text-xl font-bold text-white mb-4">
            Scan QR Code
          </h2>
          <p className="text-slate-300 mb-4">
            Scan this QR code with your authenticator app, then enter the code
            shown.
          </p>

          {/* QR Code */}
          <div className="bg-white p-4 rounded-lg mb-4 flex justify-center">
            <img
              src={setupData.qr_code_data_url}
              alt="TOTP QR Code"
              className="w-48 h-48"
            />
          </div>

          {/* Manual secret */}
          <div className="mb-4">
            <p className="text-xs text-slate-400 mb-1">
              Can't scan? Enter this code manually:
            </p>
            <code className="block bg-slate-700 p-2 rounded text-sm text-slate-300 font-mono break-all">
              {setupData.secret}
            </code>
          </div>

          {error && (
            <div className="bg-red-500/10 border border-red-500/50 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          {/* Verification form */}
          <form onSubmit={handleVerify}>
            <label className="block text-sm text-slate-300 mb-2">
              Enter verification code
            </label>
            <input
              type="text"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              className="w-full px-4 py-2.5 bg-slate-700 border border-slate-600 rounded-lg text-white text-center text-xl tracking-widest focus:outline-none focus:ring-2 focus:ring-blue-500 mb-4"
              placeholder="000000"
              maxLength={6}
              autoFocus
              autoComplete="one-time-code"
            />

            <div className="flex gap-3">
              <button
                type="button"
                onClick={onCancel}
                className="flex-1 py-2 px-4 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isLoading}
                className="flex-1 py-2 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-600/50 text-white rounded-lg transition-colors"
              >
                {isLoading ? 'Verifying...' : 'Verify'}
              </button>
            </div>
          </form>
        </div>
      </div>
    );
  }

  // Step 3: Recovery codes
  if (step === 'codes' && setupData) {
    return (
      <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
        <div className="bg-slate-800 rounded-lg shadow-xl max-w-md w-full mx-4 p-6">
          <h2 className="text-xl font-bold text-white mb-4">
            Save Recovery Codes
          </h2>
          <p className="text-slate-300 mb-4">
            Save these recovery codes in a safe place. You can use them to access
            your account if you lose your authenticator device.
          </p>

          <div className="bg-slate-700 rounded-lg p-4 mb-4">
            <div className="grid grid-cols-2 gap-2">
              {setupData.recovery_codes.map((code, i) => (
                <code key={i} className="text-sm text-slate-300 font-mono">
                  {code}
                </code>
              ))}
            </div>
          </div>

          <button
            onClick={() => {
              const text = setupData.recovery_codes.join('\n');
              navigator.clipboard.writeText(text);
            }}
            className="w-full py-2 px-4 border border-slate-600 text-slate-300 rounded-lg hover:bg-slate-700 transition-colors mb-4"
          >
            Copy to Clipboard
          </button>

          {error && (
            <div className="bg-red-500/10 border border-red-500/50 rounded-lg p-3 mb-4">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}

          <label className="flex items-center gap-2 text-sm text-slate-300 mb-4 cursor-pointer">
            <input
              type="checkbox"
              checked={codesConfirmed}
              onChange={(e) => setCodesConfirmed(e.target.checked)}
              className="w-4 h-4 rounded border-slate-600 bg-slate-700"
            />
            I have saved my recovery codes
          </label>

          <button
            onClick={handleComplete}
            disabled={!codesConfirmed}
            className="w-full py-2 px-4 bg-blue-600 hover:bg-blue-700 disabled:bg-slate-600 disabled:text-slate-400 text-white rounded-lg transition-colors"
          >
            Done
          </button>
        </div>
      </div>
    );
  }

  return null;
}
