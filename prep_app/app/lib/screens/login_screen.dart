import 'package:flutter/material.dart';

import '../api.dart';
import '../theme.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({
    super.key,
    required this.api,
    required this.setup,
    required this.onAuthenticated,
    required this.onNeedLogin,
  });

  final Api api;
  final bool setup;
  final VoidCallback onAuthenticated;
  final VoidCallback onNeedLogin;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _password = TextEditingController();
  final _confirm = TextEditingController();
  var _obscure = true;
  var _busy = false;
  String? _error;

  @override
  void dispose() {
    _password.dispose();
    _confirm.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final password = _password.text;
    final confirm = _confirm.text;
    if (widget.setup && password != confirm) {
      setState(() => _error = 'Пароли не совпадают');
      return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    var created = false;
    try {
      if (widget.setup) {
        await widget.api.setupPassword(password, confirm);
        created = true;
      }
      await widget.api.login(password);
      if (mounted) widget.onAuthenticated();
    } on ApiException catch (e) {
      if (!mounted) return;
      if (created || e.status == 409) {
        widget.onNeedLogin();
        return;
      }
      setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final wide = isWide(context);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: EdgeInsets.fromLTRB(wide ? 28 : 20, 24, wide ? 28 : 20, 32),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 420),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Text(
                    'Планка 400к+',
                    style: TextStyle(
                      fontSize: 28,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -0.6,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    widget.setup ? 'Задайте пароль' : 'Вход',
                    style: const TextStyle(color: AppColors.muted),
                  ),
                  const SizedBox(height: 22),
                  Text(
                    widget.setup
                        ? 'Пароль закрывает тренажёр в сети. На сервере остаётся только хеш Argon2id, в отдельном файле. Карточки, прогресс и ключ OpenRouter эта операция не трогает.'
                        : 'Сессия живёт 7 дней в HttpOnly-cookie. Сам пароль на диск не пишется.',
                    style: const TextStyle(height: 1.4),
                  ),
                  if (widget.setup) ...[
                    const SizedBox(height: 12),
                    const Text(
                      'С телефона пароль не задаётся: сначала откройте тренажёр на компьютере, где запущен сервер.',
                      style: TextStyle(color: AppColors.warn, height: 1.4),
                    ),
                  ],
                  const SizedBox(height: 22),
                  TextField(
                    controller: _password,
                    obscureText: _obscure,
                    autofocus: true,
                    autofillHints: [
                      widget.setup
                          ? AutofillHints.newPassword
                          : AutofillHints.password,
                    ],
                    textInputAction: widget.setup
                        ? TextInputAction.next
                        : TextInputAction.done,
                    onSubmitted: widget.setup ? null : (_) => _submit(),
                    decoration: InputDecoration(
                      labelText: 'Пароль',
                      suffixIcon: IconButton(
                        onPressed: () => setState(() => _obscure = !_obscure),
                        icon: Icon(
                          _obscure ? Icons.visibility_outlined : Icons.visibility_off_outlined,
                        ),
                      ),
                    ),
                  ),
                  if (widget.setup) ...[
                    const SizedBox(height: 12),
                    TextField(
                      controller: _confirm,
                      obscureText: _obscure,
                      autofillHints: const [AutofillHints.newPassword],
                      textInputAction: TextInputAction.done,
                      onSubmitted: (_) => _submit(),
                      decoration: const InputDecoration(labelText: 'Повтор'),
                    ),
                  ],
                  if (_error != null) ...[
                    const SizedBox(height: 14),
                    Text(_error!, style: const TextStyle(color: AppColors.bad)),
                  ],
                  const SizedBox(height: 18),
                  FilledButton(
                    onPressed: _busy ? null : _submit,
                    child: Text(
                      _busy
                          ? 'Проверяю…'
                          : widget.setup
                          ? 'Сохранить пароль'
                          : 'Войти',
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

Future<bool> showChangePasswordDialog(BuildContext context, Api api) async {
  final current = TextEditingController();
  final next = TextEditingController();
  final confirm = TextEditingController();
  var obscure = true;
  final changed = await showDialog<bool>(
    context: context,
    builder: (context) {
      String? error;
      var busy = false;
      return StatefulBuilder(
        builder: (context, setLocal) {
          Future<void> submit() async {
            if (next.text != confirm.text) {
              setLocal(() => error = 'Пароли не совпадают');
              return;
            }
            setLocal(() {
              busy = true;
              error = null;
            });
            try {
              await api.changePassword(
                current: current.text,
                password: next.text,
                confirm: confirm.text,
              );
              if (context.mounted) Navigator.pop(context, true);
            } on ApiException catch (e) {
              setLocal(() {
                busy = false;
                error = e.message;
              });
            }
          }

          return AlertDialog(
            backgroundColor: AppColors.surface,
            title: const Text('Новый пароль'),
            content: SingleChildScrollView(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Text(
                    'После смены все сессии закроются. Карточки, прогресс и ключ OpenRouter останутся на месте.',
                  ),
                  const SizedBox(height: 14),
                  TextField(
                    controller: current,
                    obscureText: obscure,
                    autofillHints: const [AutofillHints.password],
                    decoration: const InputDecoration(labelText: 'Текущий пароль'),
                  ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: next,
                    obscureText: obscure,
                    autofillHints: const [AutofillHints.newPassword],
                    decoration: InputDecoration(
                      labelText: 'Новый пароль',
                      suffixIcon: IconButton(
                        onPressed: () => setLocal(() => obscure = !obscure),
                        icon: Icon(
                          obscure ? Icons.visibility_outlined : Icons.visibility_off_outlined,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  TextField(
                    controller: confirm,
                    obscureText: obscure,
                    autofillHints: const [AutofillHints.newPassword],
                    decoration: const InputDecoration(labelText: 'Повтор'),
                    onSubmitted: (_) => submit(),
                  ),
                  if (error != null) ...[
                    const SizedBox(height: 12),
                    Text(error!, style: const TextStyle(color: AppColors.bad)),
                  ],
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: busy ? null : () => Navigator.pop(context, false),
                child: const Text('Отмена'),
              ),
              FilledButton(
                onPressed: busy ? null : submit,
                child: Text(busy ? 'Сохраняю…' : 'Сменить'),
              ),
            ],
          );
        },
      );
    },
  );
  current.dispose();
  next.dispose();
  confirm.dispose();
  return changed ?? false;
}
