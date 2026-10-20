window.addEventListener('load', function () {
  var roleNames = {
    admin: 'Admin', warden: 'Warden', sub_warden: 'Sub-Warden',
    security_staff: 'Security', student: 'Student', scanner: 'Scanner',
  };
  function rolesFor(operation) {
    var roles = operation && operation.get('x-roles');
    return roles ? roles.toArray() : [];
  }
  function RoleAccessPlugin(system) {
    var React = system.React;
    return {
      statePlugins: {
        layout: {
          actions: { setRoleFilter: function (role) { return { type: 'HOSTELHIVE_ROLE_FILTER', payload: role }; } },
          reducers: { HOSTELHIVE_ROLE_FILTER: function (state, action) { return state.set('roleFilter', action.payload); } },
          selectors: { roleFilter: function (state) { return state.get('roleFilter', 'all'); } },
        },
        spec: {
          wrapSelectors: {
            taggedOperations: function (original, runtime) {
              return function () {
                var groups = original.apply(null, arguments);
                var role = runtime.layoutSelectors.roleFilter();
                if (role === 'all') return groups;
                return groups.map(function (group) {
                  return group.set('operations', group.get('operations').filter(function (item) {
                    var roles = rolesFor(item.get('operation'));
                    return role === 'public' ? roles.length === 0 : roles.indexOf(role) !== -1;
                  }));
                }).filter(function (group) { return group.get('operations').size > 0; });
              };
            },
          },
        },
      },
      wrapComponents: {
        OperationSummary: function (Original, runtime) {
          return function (props) {
            var operation = props.operationProps;
            var spec = runtime.specSelectors.specJson();
            var roles = rolesFor(spec.getIn(['paths', operation.get('path'), operation.get('method')]));
            var labels = roles.length ? roles : ['public'];
            return React.createElement('div', { className: 'hh-operation-summary' },
              React.createElement(Original, props),
              React.createElement('div', { className: 'hh-role-badges', 'aria-label': 'Allowed roles' },
                labels.map(function (role) {
                  return React.createElement('span', {
                    className: 'hh-role-badge hh-role-' + role,
                    key: role,
                    title: role === 'public' ? 'No authentication required' : 'Available to ' + roleNames[role] + '; additional account and resource rules apply',
                  }, roleNames[role] || 'Public');
                })));
          };
        },
      },
    };
  }
  window.ui = SwaggerUIBundle({
    url: '/openapi.json',
    dom_id: '#swagger-ui',
    deepLinking: true,
    filter: true,
    docExpansion: 'none',
    defaultModelsExpandDepth: 0,
    displayRequestDuration: true,
    operationsSorter: 'alpha',
    validatorUrl: null,
    persistAuthorization: false,
    queryConfigEnabled: false,
    withCredentials: false,
    plugins: [RoleAccessPlugin],
    requestSnippetsEnabled: true,
    requestSnippets: {
      generators: {
        curl_bash: { title: 'cURL (bash)', syntax: 'bash' },
        curl_powershell: { title: 'cURL (PowerShell)', syntax: 'powershell' },
      },
      defaultExpanded: false,
    },
  });
  document.getElementById('role-filter').addEventListener('change', function (event) {
    window.ui.getSystem().layoutActions.setRoleFilter(event.target.value);
  });
});
