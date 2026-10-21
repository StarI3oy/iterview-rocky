import 'package:flutter/material.dart';

import 'api.dart';
import 'screens/dashboard.dart';
import 'screens/import_screen.dart';
import 'screens/login_screen.dart';
import 'screens/search_screen.dart';
import 'screens/settings_screen.dart';
import 'screens/trainer.dart';
import 'session.dart';
import 'theme.dart';

void main() {
  final session = Session();
  final raw = Uri.base.queryParameters['item'];
  final id = int.tryParse(raw ?? '');
  if (id != null) {
    session.focusId = id;
    session.tab = 1;
    session.epoch = 1;
  }
  runApp(PrepApp(session: session));
}

class PrepApp extends StatefulWidget {
  const PrepApp({super.key, required this.session});

  final Session session;

  @override
  State<PrepApp> createState() => _PrepAppState();
}

enum _Gate { loading, offline, setup, login, ready }

class _PrepAppState extends State<PrepApp> {
  late final Api _api = Api();
  final _navigatorKey = GlobalKey<NavigatorState>();
  _Gate _gate = _Gate.loading;
  String? _gateError;

  @override
  void initState() {
    super.initState();
    _api.onUnauthorized = () {
      if (!mounted || _gate != _Gate.ready) return;
      setState(() => _gate = _Gate.login);
    };
    _refreshAuth();
  }

  @override
  void dispose() {
    _api.client.close();
    super.dispose();
  }

  Future<void> _refreshAuth() async {
    if (_gate != _Gate.loading) {
      setState(() {
        _gate = _Gate.loading;
        _gateError = null;
      });
    }
    try {
      final status = await _api.authStatus();
      if (!mounted) return;
      setState(() {
        if (status.authenticated) {
          _gate = _Gate.ready;
        } else if (!status.passwordSet) {
          _gate = _Gate.setup;
        } else {
          _gate = _Gate.login;
        }
      });
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _gate = _Gate.offline;
        _gateError = e.message;
      });
    }
  }

  Future<void> _logout() async {
    try {
      await _api.logout();
    } on ApiException {
      // Сессия уже сброшена локально.
    }
    if (!mounted) return;
    setState(() => _gate = _Gate.login);
  }

  Future<void> _changePassword() async {
    final nav = _navigatorKey.currentContext;
    if (nav == null) return;
    final changed = await showChangePasswordDialog(nav, _api);
    if (changed && mounted) setState(() => _gate = _Gate.login);
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      navigatorKey: _navigatorKey,
      title: 'Планка 400к+',
      debugShowCheckedModeBanner: false,
      theme: buildTheme(),
      home: switch (_gate) {
        _Gate.loading => const Scaffold(
          body: Center(
            child: CircularProgressIndicator(color: AppColors.accent),
          ),
        ),
        _Gate.offline => _Offline(
          message: _gateError ?? 'Нет связи с API',
          onRetry: _refreshAuth,
        ),
        _Gate.setup => LoginScreen(
          api: _api,
          setup: true,
          onAuthenticated: () => setState(() => _gate = _Gate.ready),
          onNeedLogin: () => setState(() => _gate = _Gate.login),
        ),
        _Gate.login => LoginScreen(
          api: _api,
          setup: false,
          onAuthenticated: () => setState(() => _gate = _Gate.ready),
          onNeedLogin: () => setState(() => _gate = _Gate.login),
        ),
        _Gate.ready => ListenableBuilder(
          listenable: widget.session,
          builder: (context, _) {
            final wide = isWide(context);
            final body = switch (widget.session.tab) {
              0 => DashboardScreen(api: _api, session: widget.session),
              1 => TrainerScreen(
                key: ValueKey(widget.session.epoch),
                api: _api,
                session: widget.session,
              ),
              2 => SearchScreen(api: _api, session: widget.session),
              3 => ImportScreen(
                api: _api,
                onChangePassword: _changePassword,
                onLogout: _logout,
              ),
              _ => SettingsScreen(
                api: _api,
                onChangePassword: _changePassword,
                onLogout: _logout,
              ),
            };
            if (wide) {
              return Scaffold(
                body: SafeArea(
                  child: Row(
                    children: [
                      _SideNav(
                        session: widget.session,
                        onChangePassword: _changePassword,
                        onLogout: _logout,
                      ),
                      const VerticalDivider(width: 1, color: AppColors.line),
                      Expanded(child: body),
                    ],
                  ),
                ),
              );
            }
            return Scaffold(
              body: SafeArea(child: body),
              bottomNavigationBar: NavigationBar(
                backgroundColor: AppColors.surface,
                indicatorColor: AppColors.surface2,
                selectedIndex: widget.session.tab.clamp(0, 4),
                onDestinationSelected: (index) {
                  if (index == 0) widget.session.showHome();
                  if (index == 1) widget.session.openSprint();
                  if (index == 2) widget.session.showSearch();
                  if (index == 3) widget.session.showImport();
                  if (index == 4) widget.session.showSettings();
                },
                destinations: const [
                  NavigationDestination(icon: Icon(Icons.speed), label: 'Планка'),
                  NavigationDestination(icon: Icon(Icons.bolt), label: 'Спринт'),
                  NavigationDestination(icon: Icon(Icons.search), label: 'Поиск'),
                  NavigationDestination(
                    icon: Icon(Icons.upload_file),
                    label: 'Импорт',
                  ),
                  NavigationDestination(icon: Icon(Icons.tune), label: 'Настройки'),
                ],
              ),
            );
          },
        ),
      },
    );
  }
}

class _Offline extends StatelessWidget {
  const _Offline({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(message, textAlign: TextAlign.center),
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: onRetry,
                  child: const Text('Повторить'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _SideNav extends StatelessWidget {
  const _SideNav({
    required this.session,
    required this.onChangePassword,
    required this.onLogout,
  });

  final Session session;
  final VoidCallback onChangePassword;
  final VoidCallback onLogout;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 212,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              child: Text(
                '400k+',
                style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700),
              ),
            ),
            const SizedBox(height: 8),
            _NavButton(
              label: 'Планка',
              selected: session.tab == 0,
              onTap: session.showHome,
            ),
            _NavButton(
              label: 'Спринт',
              selected: session.tab == 1,
              onTap: session.openSprint,
            ),
            _NavButton(
              label: 'Поиск',
              selected: session.tab == 2,
              onTap: session.showSearch,
            ),
            _NavButton(
              label: 'Импорт',
              selected: session.tab == 3,
              onTap: session.showImport,
            ),
            _NavButton(
              label: 'Настройки',
              selected: session.tab == 4,
              onTap: session.showSettings,
            ),
            const Spacer(),
            _NavButton(
              label: 'Сменить пароль',
              selected: false,
              onTap: onChangePassword,
            ),
            _NavButton(label: 'Выйти', selected: false, onTap: onLogout),
            const SizedBox(height: 8),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 10),
              child: Text(
                'локальный тренажёр',
                style: TextStyle(color: AppColors.muted, fontSize: 12),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _NavButton extends StatelessWidget {
  const _NavButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Material(
        color: selected ? AppColors.surface2 : Colors.transparent,
        borderRadius: BorderRadius.circular(10),
        child: InkWell(
          borderRadius: BorderRadius.circular(10),
          onTap: onTap,
          child: Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(10),
              border: Border.all(
                color: selected ? AppColors.line : Colors.transparent,
              ),
            ),
            child: Text(
              label,
              style: TextStyle(
                fontWeight: selected ? FontWeight.w700 : FontWeight.w500,
              ),
            ),
          ),
        ),
      ),
    );
  }
}
